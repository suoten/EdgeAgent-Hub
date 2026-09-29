#!/usr/bin/env python3
"""
EdgeAgent Hub - ONNX 推理服务
提供 ONNX Runtime 推理能力，通过 NATS 接收推理请求。

启动: python inference_onnx.py --nats-url nats://127.0.0.1:4222 --model-dir ./models/onnx
"""

import argparse
import asyncio
import json
import logging
import os
import sys
import time
import uuid
from pathlib import Path
from typing import Any, Dict, List, Optional
from collections import deque

try:
    import onnxruntime as ort
except ImportError:
    print("onnxruntime not installed. Install with: pip install onnxruntime")
    sys.exit(1)

try:
    import nats
    from nats.aio.client import Client as NATSClient
except ImportError:
    print("nats-py not installed. Install with: pip install nats-py")
    sys.exit(1)

# ── 日志配置 ──
logging.basicConfig(
    level=logging.INFO,
    format='{"timestamp": "%(asctime)s", "level": "%(levelname)s", "service": "onnx-inference", "message": "%(message)s"}',
    datefmt="%Y-%m-%dT%H:%M:%S%z",
)
logger = logging.getLogger("onnx-inference")


class ONNXInferenceService:
    """ONNX 推理服务，管理多个模型的加载和推理。"""

    def __init__(self, model_dir: str, execution_provider: str = "cpu", 
                 batch_window: float = 0.05, max_batch_size: int = 8):
        self.model_dir = Path(model_dir)
        self.execution_provider = self._select_provider(execution_provider)
        self.models: Dict[str, ort.InferenceSession] = {}
        self.model_info: Dict[str, Dict] = {}
        self._load_models()
        self._nc: Optional[NATSClient] = None
        
        # 批处理参数
        self.batch_window = batch_window  # 批处理窗口 (秒)
        self.max_batch_size = max_batch_size
        self._batch_queues: Dict[str, deque] = {}  # model_id -> 请求队列
        self._batch_locks: Dict[str, asyncio.Lock] = {}
        
        # 模型预热
        self._warmup_models()

    def _select_provider(self, provider: str) -> list:
        """选择 Execution Provider，按优先级尝试。"""
        available = ort.get_available_providers()
        logger.info(f"Available ONNX Runtime providers: {available}")

        provider_map = {
            "cuda": ["CUDAExecutionProvider", "CPUExecutionProvider"],
            "tensorrt": ["TensorrtExecutionProvider", "CUDAExecutionProvider", "CPUExecutionProvider"],
            "openvino": ["OpenVINOExecutionProvider", "CPUExecutionProvider"],
            "rknn": ["RknneXecutionProvider", "CPUExecutionProvider"],
            "cpu": ["CPUExecutionProvider"],
        }

        selected = provider_map.get(provider, ["CPUExecutionProvider"])
        # 过滤不可用的 provider
        result = [p for p in selected if p in available]
        if not result:
            result = ["CPUExecutionProvider"]
        logger.info(f"Selected execution providers: {result}")
        return result

    def _load_models(self):
        """加载模型目录下所有 .onnx 模型。"""
        if not self.model_dir.exists():
            logger.warning(f"Model directory does not exist: {self.model_dir}")
            self.model_dir.mkdir(parents=True, exist_ok=True)
            return

        for model_file in self.model_dir.glob("*.onnx"):
            try:
                model_id = model_file.stem
                session = ort.InferenceSession(
                    str(model_file),
                    providers=self.execution_provider,
                )
                self.models[model_id] = session
                self.model_info[model_id] = {
                    "id": model_id,
                    "name": model_id,
                    "file": str(model_file),
                    "size_bytes": model_file.stat().st_size,
                    "inputs": [inp.name for inp in session.get_inputs()],
                    "outputs": [out.name for out in session.get_outputs()],
                    "providers": session.get_providers(),
                }
                logger.info(f"Loaded model: {model_id} ({model_file.stat().st_size} bytes)")
            except Exception as e:
                logger.error(f"Failed to load model {model_file}: {e}")

    def load_model(self, model_id: str, file_path: str) -> Dict:
        """热加载单个模型。"""
        try:
            session = ort.InferenceSession(file_path, providers=self.execution_provider)
            self.models[model_id] = session
            self.model_info[model_id] = {
                "id": model_id,
                "name": model_id,
                "file": file_path,
                "size_bytes": os.path.getsize(file_path),
                "inputs": [inp.name for inp in session.get_inputs()],
                "outputs": [out.name for out in session.get_outputs()],
                "providers": session.get_providers(),
            }
            logger.info(f"Hot-loaded model: {model_id}")
            return {"status": "loaded", "model_id": model_id}
        except Exception as e:
            logger.error(f"Failed to load model {model_id}: {e}")
            return {"status": "error", "error": str(e)}

    def unload_model(self, model_id: str) -> Dict:
        """卸载模型。"""
        if model_id in self.models:
            del self.models[model_id]
            del self.model_info[model_id]
            logger.info(f"Unloaded model: {model_id}")
            return {"status": "unloaded", "model_id": model_id}
        return {"status": "not_found", "model_id": model_id}

    def infer(self, model_id: str, inputs: Dict[str, Any]) -> Dict:
        """执行推理。"""
        if model_id not in self.models:
            return {"error": f"model not loaded: {model_id}"}

        session = self.models[model_id]
        start_time = time.time()

        try:
            # 准备输入
            input_feed = {}
            for inp in session.get_inputs():
                if inp.name in inputs:
                    input_feed[inp.name] = inputs[inp.name]
                else:
                    return {"error": f"missing input: {inp.name}"}

            # 执行推理
            results = session.run(None, input_feed)
            latency_ms = (time.time() - start_time) * 1000

            # 构建输出
            output_dict = {}
            for i, out in enumerate(session.get_outputs()):
                output_dict[out.name] = results[i].tolist() if hasattr(results[i], "tolist") else results[i]

            # 推理结果
            return {
                "model_id": model_id,
                "agent_id": "onnx.inference",
                "confidence": output_dict.get("confidence", [0.5])[0] if isinstance(output_dict.get("confidence"), list) else 0.5,
                "prediction": output_dict.get("prediction", "unknown"),
                "outputs": output_dict,
                "latency_ms": round(latency_ms, 2),
            }
        except Exception as e:
            logger.error(f"Inference error for model {model_id}: {e}")
            return {"error": str(e), "model_id": model_id}

    def list_models(self) -> Dict:
        """列出已加载的模型。"""
        return {"models": list(self.model_info.values())}

    def _warmup_models(self):
        """模型预热: 对每个已加载模型执行一次 dummy 推理，触发内部图优化。"""
        for model_id, session in self.models.items():
            try:
                inputs_info = session.get_inputs()
                dummy_inputs = {}
                for inp in inputs_info:
                    shape = inp.shape
                    # 将动态维度 (-1, None, 'batch') 替换为 1
                    fixed_shape = [1 if (not isinstance(s, int) or s < 0) else s for s in shape]
                    import numpy as np
                    dummy_inputs[inp.name] = np.zeros(fixed_shape, dtype=np.float32)
                
                session.run(None, dummy_inputs)
                logger.info(f"Model warmup completed: {model_id}")
            except Exception as e:
                logger.warning(f"Model warmup skipped for {model_id}: {e}")

    async def _batch_infer(self, model_id: str, inputs: Dict[str, Any], 
                           msg) -> Dict:
        """批处理推理: 在窗口内合并同模型的推理请求。"""
        if model_id not in self._batch_queues:
            self._batch_queues[model_id] = deque()
            self._batch_locks[model_id] = asyncio.Lock()
        
        queue = self._batch_queues[model_id]
        lock = self._batch_locks[model_id]
        
        # 创建 future 用于异步等待结果
        future = asyncio.get_event_loop().create_future()
        queue.append({
            'inputs': inputs,
            'msg': msg,
            'future': future,
        })
        
        # 如果队列达到最大批量，立即处理
        async with lock:
            if len(queue) >= self.max_batch_size:
                await self._process_batch(model_id)
            elif len(queue) == 1:
                # 启动窗口计时器
                asyncio.create_task(self._batch_timer(model_id))
        
        return await future

    async def _batch_timer(self, model_id: str):
        """批处理窗口计时器。"""
        await asyncio.sleep(self.batch_window)
        if model_id in self._batch_locks:
            async with self._batch_locks[model_id]:
                if len(self._batch_queues.get(model_id, deque())) > 0:
                    await self._process_batch(model_id)

    async def _process_batch(self, model_id: str):
        """处理一批推理请求。"""
        queue = self._batch_queues.get(model_id, deque())
        if not queue:
            return
        
        batch = list(queue)
        queue.clear()
        
        if len(batch) == 1:
            # 单个请求直接推理
            req = batch[0]
            result = self.infer(model_id, req['inputs'])
            if not req['future'].done():
                req['future'].set_result(result)
            try:
                await req['msg'].respond(json.dumps(result).encode())
            except Exception:
                pass
        else:
            # 批量推理
            logger.info(f"Batch inference: {len(batch)} requests for model {model_id}")
            for req in batch:
                result = self.infer(model_id, req['inputs'])
                if not req['future'].done():
                    req['future'].set_result(result)
                try:
                    await req['msg'].respond(json.dumps(result).encode())
                except Exception:
                    pass

    async def connect_nats(self, nats_url: str):
        """连接到 NATS 并订阅推理请求。"""
        self._nc = await nats.connect(nats_url, name=f"onnx-inference-{uuid.uuid4().hex[:8]}")
        logger.info(f"Connected to NATS: {nats_url}")

        # 订阅推理请求
        await self._nc.subscribe("agent.onnx.inference.invoke", cb=self._handle_inference_request)
        await self._nc.subscribe("agent.onnx.load", cb=self._handle_load_request)
        await self._nc.subscribe("agent.onnx.unload", cb=self._handle_unload_request)
        await self._nc.subscribe("agent.onnx.list", cb=self._handle_list_request)

        logger.info("Subscribed to ONNX inference subjects")

    async def _handle_inference_request(self, msg):
        """处理推理请求。"""
        try:
            data = json.loads(msg.data)
            model_id = data.get("model_id", "")
            inputs = data.get("inputs", {})

            # 如果没有指定 model_id, 尝试从 agent_id 提取
            if not model_id and "agent_id" in data:
                model_id = data["agent_id"].replace("onnx.", "")

            result = self.infer(model_id, inputs)
            await msg.respond(json.dumps(result).encode())
        except Exception as e:
            logger.error(f"Error handling inference request: {e}")
            await msg.respond(json.dumps({"error": str(e)}).encode())

    async def _handle_load_request(self, msg):
        """处理模型加载请求。"""
        try:
            data = json.loads(msg.data)
            result = self.load_model(data["model_id"], data["file_path"])
            await msg.respond(json.dumps(result).encode())
        except Exception as e:
            await msg.respond(json.dumps({"error": str(e)}).encode())

    async def _handle_unload_request(self, msg):
        """处理模型卸载请求。"""
        try:
            data = json.loads(msg.data)
            result = self.unload_model(data["model_id"])
            await msg.respond(json.dumps(result).encode())
        except Exception as e:
            await msg.respond(json.dumps({"error": str(e)}).encode())

    async def _handle_list_request(self, msg):
        """处理模型列表请求。"""
        result = self.list_models()
        await msg.respond(json.dumps(result).encode())

    async def disconnect(self):
        """断开 NATS 连接。"""
        if self._nc:
            await self._nc.drain()
            logger.info("Disconnected from NATS")

    async def heartbeat(self, nc):
        """定期发送心跳到 NATS。"""
        while True:
            try:
                await nc.publish("agent.heartbeat.onnx.inference", json.dumps({
                    "agent_id": "onnx.inference",
                    "type": "onnx",
                    "status": "online",
                    "timestamp": time.time(),
                }).encode())
            except Exception:
                pass
            await asyncio.sleep(10)


async def main():
    parser = argparse.ArgumentParser(description="EdgeAgent Hub ONNX Inference Service")
    parser.add_argument("--nats-url", default="nats://127.0.0.1:4222", help="NATS server URL")
    parser.add_argument("--model-dir", default="./models/onnx", help="Model directory")
    parser.add_argument("--provider", default="cpu", help="Execution provider: cpu/cuda/tensorrt/openvino")
    parser.add_argument("--batch-window", type=float, default=0.05, help="Batch window in seconds")
    parser.add_argument("--max-batch-size", type=int, default=8, help="Max batch size")
    args = parser.parse_args()

    service = ONNXInferenceService(args.model_dir, args.provider, 
                                   args.batch_window, args.max_batch_size)

    await service.connect_nats(args.nats_url)

    # 启动心跳
    if service._nc:
        asyncio.create_task(service.heartbeat(service._nc))

    logger.info(f"ONNX inference service ready ({len(service.models)} models loaded)")

    # 保持运行
    try:
        while True:
            await asyncio.sleep(1)
    except (KeyboardInterrupt, asyncio.CancelledError):
        logger.info("Shutting down...")
        await service.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
