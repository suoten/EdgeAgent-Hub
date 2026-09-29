package messaging

import (
	"testing"
)

func TestMQTTTopicToNATSSubject(t *testing.T) {
	tests := []struct {
		topic   string
		subject string
	}{
		{"sensors/vibration/line1", "edgelite.sensors.vibration.line1"},
		{"sensors/+/line1", "edgelite.sensors.*.line1"},
		{"alerts/#", "edgelite.alerts.>"},
		{"devices/device1/telemetry", "edgelite.devices.device1.telemetry"},
	}

	for _, tt := range tests {
		got := mqttTopicToNATSSubject(tt.topic)
		if got != tt.subject {
			t.Errorf("mqttTopicToNATSSubject(%q) = %q, want %q", tt.topic, got, tt.subject)
		}
	}
}

func TestMQTTTopicToNATSSubject_Basic(t *testing.T) {
	subject := mqttTopicToNATSSubject("sensors/vibration/line1")
	expected := "edgelite.sensors.vibration.line1"
	if subject != expected {
		t.Errorf("expected %s, got %s", expected, subject)
	}
}

func TestMQTTTopicToNATSSubject_Wildcards(t *testing.T) {
	// + → *
	subject := mqttTopicToNATSSubject("sensors/+/line1")
	if subject != "edgelite.sensors.*.line1" {
		t.Errorf("expected edgelite.sensors.*.line1, got %s", subject)
	}

	// # → >
	subject = mqttTopicToNATSSubject("alerts/#")
	if subject != "edgelite.alerts.>" {
		t.Errorf("expected edgelite.alerts.>, got %s", subject)
	}
}

func TestNATSSubjectToMQTTTopic_Basic(t *testing.T) {
	topic := natsSubjectToMQTTTopic("edgelite.sensors.vibration.line1")
	expected := "sensors/vibration/line1"
	if topic != expected {
		t.Errorf("expected %s, got %s", expected, topic)
	}
}

func TestNATSSubjectToMQTTTopic_Wildcards(t *testing.T) {
	// * → +
	topic := natsSubjectToMQTTTopic("edgelite.sensors.*.line1")
	if topic != "sensors/+/line1" {
		t.Errorf("expected sensors/+/line1, got %s", topic)
	}

	// > → #
	topic = natsSubjectToMQTTTopic("edgelite.alerts.>")
	if topic != "alerts/#" {
		t.Errorf("expected alerts/#, got %s", topic)
	}
}

func TestNATSSubjectToMQTTTopic_NoPrefix(t *testing.T) {
	// 没有 edgelite. 前缀的情况
	topic := natsSubjectToMQTTTopic("custom.subject.path")
	if topic != "custom/subject/path" {
		t.Errorf("expected custom/subject/path, got %s", topic)
	}
}
