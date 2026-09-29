package agent

import (
	"testing"
	"time"

	"github.com/edgelite/edgeagent-hub/internal/models"
	"github.com/sirupsen/logrus"
)

func setupTestRegistry() *Registry {
	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.DebugLevel)
	return NewRegistry(logger, 30*time.Second)
}

func TestRegisterAndDeregister(t *testing.T) {
	r := setupTestRegistry()

	agent := &models.AgentInfo{
		ID:           "onnx.test",
		Type:         models.AgentTypeONNX,
		Endpoint:     "http://localhost:50052",
		Capabilities: []string{"onnx", "inference"},
		Status:       "online",
		Weight:       1,
	}

	r.Register(agent)

	// Get
	retrieved, ok := r.Get("onnx.test")
	if !ok {
		t.Fatal("expected agent to be registered")
	}
	if retrieved.ID != "onnx.test" {
		t.Errorf("expected ID onnx.test, got %s", retrieved.ID)
	}

	// List
	agents := r.List()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}

	// Deregister
	r.Deregister("onnx.test")
	_, ok = r.Get("onnx.test")
	if ok {
		t.Error("agent should be deregistered")
	}
}

func TestDiscoverByCapability(t *testing.T) {
	r := setupTestRegistry()

	r.Register(&models.AgentInfo{
		ID:           "onnx.vibration",
		Type:         models.AgentTypeONNX,
		Capabilities: []string{"onnx", "vibration"},
		Status:       "online",
	})
	r.Register(&models.AgentInfo{
		ID:           "llm.rag",
		Type:         models.AgentTypeLLM,
		Capabilities: []string{"llm", "rag"},
		Status:       "online",
	})

	// Discover by capability
	agents := r.DiscoverByCapability("vibration")
	if len(agents) != 1 {
		t.Errorf("expected 1 agent with vibration capability, got %d", len(agents))
	}

	agents = r.DiscoverByCapability("rag")
	if len(agents) != 1 {
		t.Errorf("expected 1 agent with rag capability, got %d", len(agents))
	}

	agents = r.DiscoverByCapability("nonexistent")
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestDiscoverByType(t *testing.T) {
	r := setupTestRegistry()

	r.Register(&models.AgentInfo{
		ID:   "onnx.1",
		Type: models.AgentTypeONNX,
		Status: "online",
	})
	r.Register(&models.AgentInfo{
		ID:   "onnx.2",
		Type: models.AgentTypeONNX,
		Status: "online",
	})
	r.Register(&models.AgentInfo{
		ID:   "llm.1",
		Type: models.AgentTypeLLM,
		Status: "online",
	})

	agents := r.DiscoverByType(models.AgentTypeONNX)
	if len(agents) != 2 {
		t.Errorf("expected 2 ONNX agents, got %d", len(agents))
	}

	agents = r.DiscoverByType(models.AgentTypeLLM)
	if len(agents) != 1 {
		t.Errorf("expected 1 LLM agent, got %d", len(agents))
	}
}

func TestHeartbeat(t *testing.T) {
	r := setupTestRegistry()

	r.Register(&models.AgentInfo{
		ID:     "test.agent",
		Type:   models.AgentTypeONNX,
		Status: "online",
	})

	// Update heartbeat
	r.Heartbeat("test.agent")

	agent, _ := r.Get("test.agent")
	if agent.Status != "online" {
		t.Errorf("expected status online, got %s", agent.Status)
	}
}

func TestHealthCheck(t *testing.T) {
	r := setupTestRegistry()
	r.heartbeatTimeout = 100 * time.Millisecond

	r.Register(&models.AgentInfo{
		ID:     "test.agent",
		Type:   models.AgentTypeONNX,
		Status: "online",
	})

	// 等待超时
	time.Sleep(150 * time.Millisecond)
	r.checkHealth()

	agent, _ := r.Get("test.agent")
	if agent.Status != "offline" {
		t.Errorf("expected status offline after timeout, got %s", agent.Status)
	}
}

func TestOnlineCount(t *testing.T) {
	r := setupTestRegistry()

	r.Register(&models.AgentInfo{ID: "a1", Type: models.AgentTypeONNX, Status: "online"})
	r.Register(&models.AgentInfo{ID: "a2", Type: models.AgentTypeONNX, Status: "online"})
	r.Register(&models.AgentInfo{ID: "a3", Type: models.AgentTypeONNX, Status: "offline"})

	if r.OnlineCount() != 2 {
		t.Errorf("expected 2 online agents, got %d", r.OnlineCount())
	}
}
