#!/usr/bin/env python3
"""
EdgeAgent Hub - LLM 推理 + RAG 锚定服务
使用 llama.cpp (通过 llama-cpp-python) 进行本地 LLM 推理，
结合 RAG 锚定四步法生成可验证的告警消息。

启动: python inference_llm.py --nats-url nats://127.0.0.1:4222 --model-path ./models/llm/qwen3-0.6b-q4_k_m.gguf
"""

import argparse
import asyncio
import json
import logging
import os
import re
import sys
import time
import uuid
from pathlib import Path
from typing import Any, Dict, List, Optional

try:
    import nats
except ImportError:
    print("nats-py not installed. Install with: pip install nats-py")
    sys.exit(1)

try:
    from llama_cpp import Llama
except ImportError:
    print("llama-cpp-python not installed. Install with: pip install llama-cpp-python")
    sys.exit(1)

# ── 日志配置 ──
logging.basicConfig(
    level=logging.INFO,
    format='{"timestamp": "%(asctime)s", "level": "%(levelname)s", "service": "llm-rag", "message": "%(message)s"}',
    datefmt="%Y-%m-%dT%H:%M:%S%z",
)
logger = logging.getLogger("llm-rag")


class LLMRAGService:
    """LLM 推理 + RAG 锚定服务。"""

    # 系统提示词约束，防止 LLM 越狱
    SYSTEM_PROMPT = """你是一个工业设备运维告警助手。你的职责是:
1. 根据传感器数据和检索到的知识，生成准确、简洁的告警消息。
2. 引用的数据必须与提供的传感器读数完全一致。
3. 引用的标准和历史记录必须来自检索结果，不要编造。
4. 告警消息格式: [设备] [异常描述] [数据值] [标准引用] [建议措施]
5. 如果信息不足，明确说明"信息不足"。
不要输出与设备运维无关的内容。"""

    def __init__(
        self,
        model_path: str,
        context_size: int = 2048,
        max_tokens: int = 512,
        temperature: float = 0.3,
        gpu_layers: int = 0,
        knowledge_dir: str = "./data/knowledge",
        vectordb_dir: str = "./data/vectordb",
        embedding_dim: int = 384,
        top_k: int = 5,
        anchor_threshold: float = 0.85,
    ):
        self.model_path = model_path
        self.context_size = context_size
        self.max_tokens = max_tokens
        self.temperature = temperature
        self.top_k = top_k
        self.anchor_threshold = anchor_threshold
        self.knowledge_dir = Path(knowledge_dir)
        self.vectordb_dir = Path(vectordb_dir)
        self.embedding_dim = embedding_dim

        # 知识库 (简化: 文本索引)
        self.knowledge_docs: List[Dict] = []
        self._load_knowledge()

        # 加载 LLM 模型
        self.llm: Optional[Llama] = None
        self._load_model()

        self._nc = None

    def _load_model(self):
        """加载 LLM 模型。"""
        if not os.path.exists(self.model_path):
            logger.warning(f"LLM model not found: {self.model_path}, LLM disabled")
            return

        try:
            self.llm = Llama(
                model_path=self.model_path,
                n_ctx=self.context_size,
                n_gpu_layers=self.gpu_layers if hasattr(self, 'gpu_layers') else 0,
                verbose=False,
            )
            logger.info(f"LLM model loaded: {self.model_path}")
        except Exception as e:
            logger.error(f"Failed to load LLM model: {e}")

    def _load_knowledge(self):
        """加载知识库文档。"""
        if not self.knowledge_dir.exists():
            logger.warning(f"Knowledge directory does not exist: {self.knowledge_dir}")
            self.knowledge_dir.mkdir(parents=True, exist_ok=True)
            return

        for doc_file in self.knowledge_dir.glob("*"):
            if doc_file.suffix in [".txt", ".md", ".json"]:
                try:
                    content = doc_file.read_text(encoding="utf-8")
                    # 简单切片: 按段落分割
                    chunks = [c.strip() for c in content.split("\n\n") if c.strip()]
                    for i, chunk in enumerate(chunks):
                        self.knowledge_docs.append({
                            "content": chunk,
                            "source": f"{doc_file.name}#{i}",
                            "keywords": self._extract_keywords(chunk),
                        })
                except Exception as e:
                    logger.error(f"Failed to load knowledge file {doc_file}: {e}")

        logger.info(f"Loaded {len(self.knowledge_docs)} knowledge chunks from {self.knowledge_dir}")

    def _extract_keywords(self, text: str) -> List[str]:
        """从文本中提取关键词 (简化版)。"""
        # 简化: 提取中文词组和英文单词
        words = re.findall(r'[\u4e00-\u9fa5]{2,}|[a-zA-Z]{3,}', text)
        return list(set(words))[:10]

    def search_knowledge(self, query: str, top_k: int = 5) -> List[Dict]:
        """知识库检索 (简化: 基于关键词匹配)。"""
        query_keywords = self._extract_keywords(query)
        scored = []
        for doc in self.knowledge_docs:
            # 简化评分: 关键词交集数量
            overlap = len(set(query_keywords) & set(doc["keywords"]))
            if overlap > 0:
                scored.append((overlap, doc))

        scored.sort(key=lambda x: x[0], reverse=True)
        return [{"content": d["content"], "source": d["source"], "score": s} for s, d in scored[:top_k]]

    def semanticize_sensor_data(self, sensor_data: Dict) -> str:
        """
        RAG 锚定步骤 1: 传感器数据语义化
        将原始传感器读数转换为自然语言描述。
        """
        metrics = sensor_data.get("metrics", {})
        device_id = sensor_data.get("device_id", "未知设备")
        descriptions = []

        for metric, value in metrics.items():
            unit = sensor_data.get("metadata", {}).get("unit", {}).get(metric, "")
            if "vibration" in metric:
                descriptions.append(f"{device_id} 振动值 {value} {unit}")
            elif "temperature" in metric:
                descriptions.append(f"{device_id} 温度 {value} {unit}")
            elif "current" in metric:
                descriptions.append(f"{device_id} 电流 {value} {unit}")
            elif "power" in metric:
                descriptions.append(f"{device_id} 功率 {value} {unit}")
            else:
                descriptions.append(f"{device_id} {metric} = {value} {unit}")

        return "；".join(descriptions)

    def generate_alert(
        self,
        sensor_data: Dict,
        anomaly_result: Optional[Dict] = None,
        vision_result: Optional[Dict] = None,
    ) -> Dict:
        """
        RAG 锚定四步法: 语义化→检索→生成→验证
        """
        # 步骤 1: 传感器数据语义化
        semanticized = self.semanticize_sensor_data(sensor_data)

        # 步骤 2: RAG 检索
        query = semanticized
        if anomaly_result and "prediction" in anomaly_result:
            query += f" {anomaly_result['prediction']}"
        rag_results = self.search_knowledge(query, self.top_k)
        rag_context = "\n".join([f"[{r['source']}] {r['content']}" for r in rag_results])
        rag_sources = [r["source"] for r in rag_results]

        # 步骤 3: LLM 生成告警
        if self.llm is None:
            # LLM 不可用，使用模板告警
            return self._fallback_alert(sensor_data, anomaly_result, rag_sources)

        prompt = self._build_prompt(semanticized, anomaly_result, vision_result, rag_context)

        try:
            response = self.llm(
                prompt,
                max_tokens=self.max_tokens,
                temperature=self.temperature,
                stop=["</s>", "[END]"],
            )
            llm_output = response["choices"][0]["text"].strip()
        except Exception as e:
            logger.error(f"LLM generation failed: {e}")
            return self._fallback_alert(sensor_data, anomaly_result, rag_sources)

        # 步骤 4: 锚定验证
        anchored, evidence = self._anchor_verify(llm_output, sensor_data, rag_results)

        if not anchored:
            logger.warning("Anchor verification failed, using fallback alert")
            return self._fallback_alert(sensor_data, anomaly_result, rag_sources)

        return {
            "message": llm_output,
            "anchored": True,
            "evidence": evidence,
            "rag_sources": rag_sources,
            "fallback_used": False,
        }

    def _build_prompt(
        self,
        semanticized: str,
        anomaly_result: Optional[Dict],
        vision_result: Optional[Dict],
        rag_context: str,
    ) -> str:
        """构建 LLM 提示词。"""
        prompt = f"{self.SYSTEM_PROMPT}\n\n"
        prompt += f"传感器数据: {semanticized}\n"

        if anomaly_result:
            prompt += f"异常检测结果: {json.dumps(anomaly_result, ensure_ascii=False)}\n"

        if vision_result:
            prompt += f"视觉确认结果: {json.dumps(vision_result, ensure_ascii=False)}\n"

        if rag_context:
            prompt += f"知识库检索结果:\n{rag_context}\n"

        prompt += "\n请生成告警消息:"
        return prompt

    def _anchor_verify(
        self,
        llm_output: str,
        sensor_data: Dict,
        rag_results: List[Dict],
    ) -> tuple:
        """
        RAG 锚定步骤 4: 验证
        检查 LLM 输出中的数值是否与传感器数据一致。
        """
        evidence = {"checks": [], "sensor_values": {}, "rag_matches": []}
        all_passed = True

        # 检查 1: 传感器数值一致性
        metrics = sensor_data.get("metrics", {})
        for metric, expected_value in metrics.items():
            # 在 LLM 输出中搜索该数值
            value_str = str(expected_value)
            if value_str in llm_output:
                evidence["checks"].append(f"✓ {metric}={expected_value} 在输出中一致")
                evidence["sensor_values"][metric] = expected_value
            else:
                # 尝试查找不匹配的数值 (幻觉检测)
                # 搜索类似格式的数值
                pattern = rf'{metric}[^0-9]*([0-9]+\.?[0-9]*)'
                match = re.search(pattern, llm_output)
                if match:
                    found_value = float(match.group(1))
                    if abs(found_value - expected_value) > 0.1:
                        evidence["checks"].append(f"✗ {metric}: 输出值 {found_value} 与实际 {expected_value} 不一致")
                        all_passed = False
                    else:
                        evidence["checks"].append(f"✓ {metric}={expected_value} 近似匹配")
                        evidence["sensor_values"][metric] = expected_value

        # 检查 2: RAG 引用存在性
        for rag in rag_results:
            source = rag["source"]
            # 检查 LLM 输出是否引用了知识库内容
            keywords = rag.get("content", "")[:20]
            if keywords and keywords in llm_output:
                evidence["rag_matches"].append(source)

        return all_passed, evidence

    def _fallback_alert(
        self,
        sensor_data: Dict,
        anomaly_result: Optional[Dict],
        rag_sources: List[str],
    ) -> Dict:
        """生成模板回退告警 (LLM 不可用或锚定失败时)。"""
        device_id = sensor_data.get("device_id", "未知设备")
        metrics = sensor_data.get("metrics", {})

        parts = [f"[ALERT] {device_id}"]
        for metric, value in metrics.items():
            unit = sensor_data.get("metadata", {}).get("unit", {}).get(metric, "")
            parts.append(f"{metric}={value}{unit}")

        if anomaly_result and "prediction" in anomaly_result:
            parts.append(f"异常: {anomaly_result['prediction']}")

        parts.append("请检查设备。")

        return {
            "message": " ".join(parts),
            "anchored": False,
            "evidence": {"fallback": True},
            "rag_sources": rag_sources,
            "fallback_used": True,
        }

    def chat(self, message: str, context: str = "") -> Dict:
        """运维问答对话。"""
        if self.llm is None:
            return {
                "response": "LLM 服务不可用，请检查模型配置。",
                "anchored": False,
            }

        # 检索相关知识
        rag_results = self.search_knowledge(message, self.top_k)
        rag_context = "\n".join([f"[{r['source']}] {r['content']}" for r in rag_results])

        prompt = f"{self.SYSTEM_PROMPT}\n\n"
        if context:
            prompt += f"上下文: {context}\n"
        if rag_context:
            prompt += f"相关知识:\n{rag_context}\n"
        prompt += f"\n用户问题: {message}\n\n回答:"

        try:
            response = self.llm(
                prompt,
                max_tokens=self.max_tokens,
                temperature=self.temperature,
                stop=["</s>", "[END]"],
            )
            answer = response["choices"][0]["text"].strip()
        except Exception as e:
            logger.error(f"Chat failed: {e}")
            answer = f"对话生成失败: {e}"

        return {
            "response": answer,
            "sources": [r["source"] for r in rag_results],
            "anchored": len(rag_results) > 0,
        }

    async def connect_nats(self, nats_url: str):
        """连接到 NATS 并订阅请求。"""
        self._nc = await nats.connect(nats_url, name=f"llm-rag-{uuid.uuid4().hex[:8]}")
        logger.info(f"Connected to NATS: {nats_url}")

        # 订阅 RAG 锚定告警请求
        await self._nc.subscribe("agent.llm.rag_anchored.invoke", cb=self._handle_rag_request)
        # 订阅对话请求
        await self._nc.subscribe("agent.llm.chat", cb=self._handle_chat_request)
        # 订阅知识库索引请求
        await self._nc.subscribe("agent.rag.index", cb=self._handle_index_request)
        # 订阅知识库检索请求
        await self._nc.subscribe("agent.rag.search", cb=self._handle_search_request)

        logger.info("Subscribed to LLM/RAG subjects")

    async def _handle_rag_request(self, msg):
        """处理 RAG 锚定告警请求。"""
        try:
            data = json.loads(msg.data)
            result = self.generate_alert(
                sensor_data=data.get("sensor_data", {}),
                anomaly_result=data.get("anomaly_result"),
                vision_result=data.get("vision_result"),
            )
            await msg.respond(json.dumps(result, ensure_ascii=False).encode())
        except Exception as e:
            logger.error(f"RAG request error: {e}")
            await msg.respond(json.dumps({"error": str(e)}).encode())

    async def _handle_chat_request(self, msg):
        """处理对话请求。"""
        try:
            data = json.loads(msg.data)
            result = self.chat(
                message=data.get("message", ""),
                context=data.get("context", ""),
            )
            await msg.respond(json.dumps(result, ensure_ascii=False).encode())
        except Exception as e:
            logger.error(f"Chat request error: {e}")
            await msg.respond(json.dumps({"error": str(e)}).encode())

    async def _handle_index_request(self, msg):
        """处理知识库索引请求。"""
        try:
            data = json.loads(msg.data)
            file_path = data.get("file_path", "")
            if os.path.exists(file_path):
                content = Path(file_path).read_text(encoding="utf-8")
                chunks = [c.strip() for c in content.split("\n\n") if c.strip()]
                for i, chunk in enumerate(chunks):
                    self.knowledge_docs.append({
                        "content": chunk,
                        "source": f"{Path(file_path).name}#{i}",
                        "keywords": self._extract_keywords(chunk),
                    })
                logger.info(f"Indexed {len(chunks)} chunks from {file_path}")
                await msg.respond(json.dumps({"status": "indexed", "chunks": len(chunks)}).encode())
            else:
                await msg.respond(json.dumps({"error": "file not found"}).encode())
        except Exception as e:
            await msg.respond(json.dumps({"error": str(e)}).encode())

    async def _handle_search_request(self, msg):
        """处理知识库检索请求。"""
        try:
            data = json.loads(msg.data)
            query = data.get("query", "")
            results = self.search_knowledge(query, self.top_k)
            await msg.respond(json.dumps({"results": results}, ensure_ascii=False).encode())
        except Exception as e:
            await msg.respond(json.dumps({"error": str(e)}).encode())

    async def disconnect(self):
        """断开 NATS 连接。"""
        if self._nc:
            await self._nc.drain()
            logger.info("Disconnected from NATS")

    async def heartbeat(self):
        """定期发送心跳。"""
        while True:
            try:
                await self._nc.publish("agent.heartbeat.llm.rag_anchored", json.dumps({
                    "agent_id": "llm.rag_anchored",
                    "type": "llm",
                    "status": "online" if self.llm else "degraded",
                    "timestamp": time.time(),
                }).encode())
            except Exception:
                pass
            await asyncio.sleep(10)


async def main():
    parser = argparse.ArgumentParser(description="EdgeAgent Hub LLM + RAG Service")
    parser.add_argument("--nats-url", default="nats://127.0.0.1:4222")
    parser.add_argument("--model-path", default="./models/llm/qwen3-0.6b-q4_k_m.gguf")
    parser.add_argument("--context-size", type=int, default=2048)
    parser.add_argument("--max-tokens", type=int, default=512)
    parser.add_argument("--temperature", type=float, default=0.3)
    parser.add_argument("--gpu-layers", type=int, default=0)
    parser.add_argument("--knowledge-dir", default="./data/knowledge")
    parser.add_argument("--vectordb-dir", default="./data/vectordb")
    parser.add_argument("--embedding-dim", type=int, default=384)
    parser.add_argument("--top-k", type=int, default=5)
    parser.add_argument("--anchor-threshold", type=float, default=0.85)
    args = parser.parse_args()

    service = LLMRAGService(
        model_path=args.model_path,
        context_size=args.context_size,
        max_tokens=args.max_tokens,
        temperature=args.temperature,
        gpu_layers=args.gpu_layers,
        knowledge_dir=args.knowledge_dir,
        vectordb_dir=args.vectordb_dir,
        embedding_dim=args.embedding_dim,
        top_k=args.top_k,
        anchor_threshold=args.anchor_threshold,
    )

    await service.connect_nats(args.nats_url)
    asyncio.create_task(service.heartbeat())

    llm_status = "ready" if service.llm else "degraded (model not loaded)"
    logger.info(f"LLM + RAG service ready ({llm_status}, {len(service.knowledge_docs)} knowledge chunks)")

    try:
        while True:
            await asyncio.sleep(1)
    except (KeyboardInterrupt, asyncio.CancelledError):
        logger.info("Shutting down...")
        await service.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
