package protocol

import (
	"fmt"
	"strings"
	"testing"
)

func TestNewRequest(t *testing.T) {
	req := NewRequest("TestService", "TestMethod", []interface{}{123})

	if req.Service != "TestService" {
		t.Errorf("Expected Service to be 'TestService', got '%s'", req.Service)
	}

	if req.Method != "TestMethod" {
		t.Errorf("Expected Method to be 'TestMethod', got '%s'", req.Method)
	}

	if len(req.Args) != 1 || req.Args[0] != 123 {
		t.Errorf("Expected Args to be [123], got %v", req.Args)
	}

	if req.ID == 0 {
		t.Errorf("Expected non-zero Request ID")
	}

	t.Logf("Created Request: %v", req)
}

func TestNewResponse(t *testing.T) {
	resp := NewResponse(1, "TestData", nil)

	if resp.ID != 1 {
		t.Errorf("Expected Response ID to be 1, got %d", resp.ID)
	}

	if resp.Data != "TestData" {
		t.Errorf("Expected Data to be 'TestData', got '%v'", resp.Data)
	}

	if resp.Error != "" {
		t.Errorf("Expected no Error, got '%s'", resp.Error)
	}

	t.Logf("Created Response: %v", resp)

	errResp := NewResponse(2, nil, fmt.Errorf("Test error"))

	if errResp.ID != 2 {
		t.Errorf("Expected Response ID to be 2, got %d", errResp.ID)
	}

	if errResp.Data != nil {
		t.Errorf("Expected Data to be nil, got '%v'", errResp.Data)
	}

	if errResp.Error != "Test error" {
		t.Errorf("Expected Error to be 'Test error', got '%s'", errResp.Error)
	}

	t.Logf("Created Error Response: %v", errResp)
}

func TestRequestString(t *testing.T) {
	req := NewRequest("TestService", "TestMethod", []interface{}{123})
	str := req.String()

	if !strings.Contains(str, "TestService") || !strings.Contains(str, "TestMethod") {
		t.Errorf("String() output incomplete: %s", str)
	}

	t.Logf("Request.String() = %s", str)
}
