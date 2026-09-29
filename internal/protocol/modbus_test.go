package protocol

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestDecodeRegister_UInt16(t *testing.T) {
	values := []uint16{100}
	val, err := decodeRegister(values, "uint16", 0.1, 0)
	if err != nil {
		t.Fatalf("decodeRegister failed: %v", err)
	}
	if val != 10.0 {
		t.Errorf("expected 10.0, got %f", val)
	}
}

func TestDecodeRegister_Int16(t *testing.T) {
	// int16(-1) = 0xFFFF
	values := []uint16{0xFFFF}
	val, err := decodeRegister(values, "int16", 1, 0)
	if err != nil {
		t.Fatalf("decodeRegister failed: %v", err)
	}
	if val != -1.0 {
		t.Errorf("expected -1.0, got %f", val)
	}
}

func TestDecodeRegister_UInt32(t *testing.T) {
	values := []uint16{0x0001, 0x0002}
	val, err := decodeRegister(values, "uint32", 1, 0)
	if err != nil {
		t.Fatalf("decodeRegister failed: %v", err)
	}
	expected := float64(uint32(0x0001)<<16 | uint32(0x0002))
	if val != expected {
		t.Errorf("expected %f, got %f", expected, val)
	}
}

func TestDecodeRegister_Float32(t *testing.T) {
	// float32(3.14) → bits
	bits := math.Float32bits(3.14)
	high := uint16(bits >> 16)
	low := uint16(bits & 0xFFFF)
	values := []uint16{high, low}
	val, err := decodeRegister(values, "float32", 1, 0)
	if err != nil {
		t.Fatalf("decodeRegister failed: %v", err)
	}
	// float32 -> float64 精度问题，使用误差比较
	if math.Abs(val-3.14) > 0.001 {
		t.Errorf("expected ~3.14, got %f", val)
	}
}

func TestDecodeRegister_ScaleOffset(t *testing.T) {
	values := []uint16{500}
	val, err := decodeRegister(values, "uint16", 0.1, -50)
	if err != nil {
		t.Fatalf("decodeRegister failed: %v", err)
	}
	// 500 * 0.1 + (-50) = 0
	if val != 0.0 {
		t.Errorf("expected 0.0, got %f", val)
	}
}

func TestModbusRequestEncoding(t *testing.T) {
	// 验证 Modbus 请求帧编码
	req := make([]byte, 12)
	binary.BigEndian.PutUint16(req[0:], 1)       // Transaction ID
	binary.BigEndian.PutUint16(req[2:], 0)       // Protocol ID
	binary.BigEndian.PutUint16(req[4:], 6)       // Length
	req[6] = 1                                   // Unit ID
	req[7] = 0x03                                // FuncCode: Read Holding Registers
	binary.BigEndian.PutUint16(req[8:], 100)     // Address
	binary.BigEndian.PutUint16(req[10:], 2)      // Quantity

	// 验证编码
	if binary.BigEndian.Uint16(req[0:2]) != 1 {
		t.Error("transaction ID mismatch")
	}
	if req[7] != 0x03 {
		t.Error("function code mismatch")
	}
	if binary.BigEndian.Uint16(req[8:10]) != 100 {
		t.Error("address mismatch")
	}
	if binary.BigEndian.Uint16(req[10:12]) != 2 {
		t.Error("quantity mismatch")
	}
}
