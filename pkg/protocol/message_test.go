package protocol

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	tests := []struct {
		name     string
		msgType  MessageType
		data     interface{}
		wantErr  bool
	}{
		{
			name:    "frame message",
			msgType: TypeFrame,
			data:    FrameData{Width: 640, Height: 480, Format: "jpeg"},
			wantErr: false,
		},
		{
			name:    "doa message",
			msgType: TypeDOA,
			data:    DOAData{Angle: 0.5, Speaking: true, Confidence: 0.9},
			wantErr: false,
		},
		{
			name:    "nil data",
			msgType: TypePing,
			data:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := NewMessage(tt.msgType, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if msg == nil && !tt.wantErr {
				t.Error("NewMessage() returned nil message")
				return
			}
			if msg.Type != tt.msgType {
				t.Errorf("NewMessage() type = %v, want %v", msg.Type, tt.msgType)
			}
			if msg.Timestamp == 0 {
				t.Error("NewMessage() timestamp should be set")
			}
		})
	}
}

func TestMessageRoundTrip(t *testing.T) {
	// Create a frame message
	originalFrame := FrameData{
		Width:   1920,
		Height:  1080,
		Format:  "jpeg",
		Data:    base64.StdEncoding.EncodeToString([]byte("test image data")),
		FrameID: 42,
	}

	msg, err := NewMessage(TypeFrame, originalFrame)
	if err != nil {
		t.Fatalf("NewMessage() error = %v", err)
	}

	// Serialize to bytes
	bytes, err := msg.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	// Parse back
	parsed, err := ParseMessage(bytes)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	// Verify type
	if parsed.Type != TypeFrame {
		t.Errorf("Type = %v, want %v", parsed.Type, TypeFrame)
	}

	// Extract data
	frameData, err := parsed.GetFrameData()
	if err != nil {
		t.Fatalf("GetFrameData() error = %v", err)
	}

	if frameData.Width != originalFrame.Width {
		t.Errorf("Width = %v, want %v", frameData.Width, originalFrame.Width)
	}
	if frameData.Height != originalFrame.Height {
		t.Errorf("Height = %v, want %v", frameData.Height, originalFrame.Height)
	}
	if frameData.FrameID != originalFrame.FrameID {
		t.Errorf("FrameID = %v, want %v", frameData.FrameID, originalFrame.FrameID)
	}
}

func TestFrameMessage(t *testing.T) {
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10} // Fake JPEG header

	msg, err := NewFrameMessage(640, 480, jpegData, 1)
	if err != nil {
		t.Fatalf("NewFrameMessage() error = %v", err)
	}

	if msg.Type != TypeFrame {
		t.Errorf("Type = %v, want %v", msg.Type, TypeFrame)
	}

	frameData, err := msg.GetFrameData()
	if err != nil {
		t.Fatalf("GetFrameData() error = %v", err)
	}

	if frameData.Width != 640 {
		t.Errorf("Width = %v, want 640", frameData.Width)
	}
	if frameData.Format != "jpeg" {
		t.Errorf("Format = %v, want jpeg", frameData.Format)
	}

	decoded, err := frameData.DecodeFrameData()
	if err != nil {
		t.Fatalf("DecodeFrameData() error = %v", err)
	}

	if len(decoded) != len(jpegData) {
		t.Errorf("Decoded length = %v, want %v", len(decoded), len(jpegData))
	}
}

func TestDOAMessage(t *testing.T) {
	msg, err := NewDOAMessage(0.5, 0.48, true, true, 0.95)
	if err != nil {
		t.Fatalf("NewDOAMessage() error = %v", err)
	}

	if msg.Type != TypeDOA {
		t.Errorf("Type = %v, want %v", msg.Type, TypeDOA)
	}

	doaData, err := msg.GetDOAData()
	if err != nil {
		t.Fatalf("GetDOAData() error = %v", err)
	}

	if doaData.Angle != 0.5 {
		t.Errorf("Angle = %v, want 0.5", doaData.Angle)
	}
	if !doaData.Speaking {
		t.Error("Speaking should be true")
	}
	if doaData.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", doaData.Confidence)
	}
}

func TestMotorMessage(t *testing.T) {
	head := HeadTarget{X: 0.1, Y: 0.2, Z: 0.3, Yaw: 0.5}
	antennas := [2]float64{0.3, 0.7}

	msg, err := NewMotorMessage(head, antennas, 0.1)
	if err != nil {
		t.Fatalf("NewMotorMessage() error = %v", err)
	}

	if msg.Type != TypeMotor {
		t.Errorf("Type = %v, want %v", msg.Type, TypeMotor)
	}

	motorCmd, err := msg.GetMotorCommand()
	if err != nil {
		t.Fatalf("GetMotorCommand() error = %v", err)
	}

	if motorCmd.Head.X != 0.1 {
		t.Errorf("Head.X = %v, want 0.1", motorCmd.Head.X)
	}
	if motorCmd.Antennas[0] != 0.3 {
		t.Errorf("Antennas[0] = %v, want 0.3", motorCmd.Antennas[0])
	}
	if motorCmd.BodyYaw != 0.1 {
		t.Errorf("BodyYaw = %v, want 0.1", motorCmd.BodyYaw)
	}
}

func TestSpeakMessage(t *testing.T) {
	audioData := []byte{0x00, 0x01, 0x02, 0x03}

	msg, err := NewSpeakMessage(audioData, "pcm16", 22050)
	if err != nil {
		t.Fatalf("NewSpeakMessage() error = %v", err)
	}

	if msg.Type != TypeSpeak {
		t.Errorf("Type = %v, want %v", msg.Type, TypeSpeak)
	}

	speakData, err := msg.GetSpeakData()
	if err != nil {
		t.Fatalf("GetSpeakData() error = %v", err)
	}

	if speakData.Format != "pcm16" {
		t.Errorf("Format = %v, want pcm16", speakData.Format)
	}
	if speakData.SampleRate != 22050 {
		t.Errorf("SampleRate = %v, want 22050", speakData.SampleRate)
	}

	decoded, err := speakData.DecodeSpeakData()
	if err != nil {
		t.Fatalf("DecodeSpeakData() error = %v", err)
	}

	if len(decoded) != len(audioData) {
		t.Errorf("Decoded length = %v, want %v", len(decoded), len(audioData))
	}
}

func TestEmotionMessage(t *testing.T) {
	msg, err := NewEmotionMessage("happy", 2.5)
	if err != nil {
		t.Fatalf("NewEmotionMessage() error = %v", err)
	}

	if msg.Type != TypeEmotion {
		t.Errorf("Type = %v, want %v", msg.Type, TypeEmotion)
	}

	emotionCmd, err := msg.GetEmotionCommand()
	if err != nil {
		t.Fatalf("GetEmotionCommand() error = %v", err)
	}

	if emotionCmd.Name != "happy" {
		t.Errorf("Name = %v, want happy", emotionCmd.Name)
	}
	if emotionCmd.Duration != 2.5 {
		t.Errorf("Duration = %v, want 2.5", emotionCmd.Duration)
	}
}

func TestConfigMessage(t *testing.T) {
	camera := &CameraConfig{
		Width:     1920,
		Height:    1080,
		Framerate: 30,
		Preset:    "1080p",
	}

	msg, err := NewConfigMessage(camera, nil)
	if err != nil {
		t.Fatalf("NewConfigMessage() error = %v", err)
	}

	if msg.Type != TypeConfig {
		t.Errorf("Type = %v, want %v", msg.Type, TypeConfig)
	}

	configUpdate, err := msg.GetConfigUpdate()
	if err != nil {
		t.Fatalf("GetConfigUpdate() error = %v", err)
	}

	if configUpdate.Camera == nil {
		t.Fatal("Camera config should not be nil")
	}
	if configUpdate.Camera.Width != 1920 {
		t.Errorf("Camera.Width = %v, want 1920", configUpdate.Camera.Width)
	}
	if configUpdate.Audio != nil {
		t.Error("Audio config should be nil")
	}
}

func TestPingPongMessage(t *testing.T) {
	pingMsg, err := NewPingMessage("test-123")
	if err != nil {
		t.Fatalf("NewPingMessage() error = %v", err)
	}

	if pingMsg.Type != TypePing {
		t.Errorf("Type = %v, want %v", pingMsg.Type, TypePing)
	}

	pingData, err := pingMsg.GetPingData()
	if err != nil {
		t.Fatalf("GetPingData() error = %v", err)
	}

	if pingData.ID != "test-123" {
		t.Errorf("ID = %v, want test-123", pingData.ID)
	}

	// Create pong response
	now := time.Now().UnixMilli()
	pongMsg, err := NewPongMessage("test-123", pingMsg.Timestamp, now)
	if err != nil {
		t.Fatalf("NewPongMessage() error = %v", err)
	}

	if pongMsg.Type != TypePong {
		t.Errorf("Type = %v, want %v", pongMsg.Type, TypePong)
	}

	pongData, err := pongMsg.GetPongData()
	if err != nil {
		t.Fatalf("GetPongData() error = %v", err)
	}

	if pongData.ID != "test-123" {
		t.Errorf("ID = %v, want test-123", pongData.ID)
	}
	if pongData.LatencyMs < 0 {
		t.Errorf("LatencyMs = %v, should be >= 0", pongData.LatencyMs)
	}
}

func TestStateMessage(t *testing.T) {
	joints := &JointState{
		NeckRoll:     0.1,
		NeckPitch:    0.2,
		NeckYaw:      0.3,
		LeftAntenna:  0.4,
		RightAntenna: 0.5,
		BodyYaw:      0.6,
	}

	msg, err := NewStateMessage(true, joints, nil)
	if err != nil {
		t.Fatalf("NewStateMessage() error = %v", err)
	}

	if msg.Type != TypeState {
		t.Errorf("Type = %v, want %v", msg.Type, TypeState)
	}

	stateData, err := msg.GetStateData()
	if err != nil {
		t.Fatalf("GetStateData() error = %v", err)
	}

	if !stateData.Connected {
		t.Error("Connected should be true")
	}
	if stateData.Joints == nil {
		t.Fatal("Joints should not be nil")
	}
	if stateData.Joints.NeckYaw != 0.3 {
		t.Errorf("NeckYaw = %v, want 0.3", stateData.Joints.NeckYaw)
	}
}

func TestMicMessage(t *testing.T) {
	pcmData := make([]byte, 1024)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}

	msg, err := NewMicMessage(pcmData, 16000)
	if err != nil {
		t.Fatalf("NewMicMessage() error = %v", err)
	}

	if msg.Type != TypeMic {
		t.Errorf("Type = %v, want %v", msg.Type, TypeMic)
	}

	micData, err := msg.GetMicData()
	if err != nil {
		t.Fatalf("GetMicData() error = %v", err)
	}

	if micData.SampleRate != 16000 {
		t.Errorf("SampleRate = %v, want 16000", micData.SampleRate)
	}
	if micData.Format != "pcm16" {
		t.Errorf("Format = %v, want pcm16", micData.Format)
	}

	decoded, err := micData.DecodeMicData()
	if err != nil {
		t.Fatalf("DecodeMicData() error = %v", err)
	}

	if len(decoded) != len(pcmData) {
		t.Errorf("Decoded length = %v, want %v", len(decoded), len(pcmData))
	}
}

func TestParseInvalidMessage(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "invalid json",
			input:   "not json",
			wantErr: true,
		},
		{
			name:    "empty json",
			input:   "{}",
			wantErr: false, // Empty is valid, just no type
		},
		{
			name:    "valid message",
			input:   `{"type":"ping","ts":1234567890}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMessage([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMessage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMessageJSON(t *testing.T) {
	// Verify JSON structure matches expected format
	msg, _ := NewMotorMessage(
		HeadTarget{X: 0.1, Y: 0.2, Z: 0.3, Yaw: 0.5},
		[2]float64{0.3, 0.7},
		0.1,
	)

	bytes, _ := msg.Bytes()

	var parsed map[string]interface{}
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal as map: %v", err)
	}

	if parsed["type"] != "motor" {
		t.Errorf("type = %v, want motor", parsed["type"])
	}

	if _, ok := parsed["ts"]; !ok {
		t.Error("ts field should be present")
	}

	if _, ok := parsed["data"]; !ok {
		t.Error("data field should be present")
	}
}

func BenchmarkNewFrameMessage(b *testing.B) {
	jpegData := make([]byte, 100*1024) // 100KB fake JPEG

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewFrameMessage(1920, 1080, jpegData, uint64(i))
	}
}

func BenchmarkParseMessage(b *testing.B) {
	msg, _ := NewFrameMessage(1920, 1080, make([]byte, 100*1024), 1)
	bytes, _ := msg.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseMessage(bytes)
	}
}

