package protocol

import (
	"encoding/base64"
)

// =============================================================================
// Helper functions for creating messages
// =============================================================================

// NewFrameMessage creates a frame message from raw JPEG data
func NewFrameMessage(width, height int, jpegData []byte, frameID uint64) (*Message, error) {
	return NewMessage(TypeFrame, FrameData{
		Width:   width,
		Height:  height,
		Format:  "jpeg",
		Data:    base64.StdEncoding.EncodeToString(jpegData),
		FrameID: frameID,
	})
}

// NewDOAMessage creates a DOA message
func NewDOAMessage(angle, smoothedAngle float64, speaking, speakingLatched bool, confidence float64) (*Message, error) {
	return NewMessage(TypeDOA, DOAData{
		Angle:           angle,
		SmoothedAngle:   smoothedAngle,
		Speaking:        speaking,
		SpeakingLatched: speakingLatched,
		Confidence:      confidence,
	})
}

// NewMicMessage creates a microphone audio message
func NewMicMessage(pcmData []byte, sampleRate int) (*Message, error) {
	return NewMessage(TypeMic, MicData{
		Format:     "pcm16",
		SampleRate: sampleRate,
		Channels:   1,
		Data:       base64.StdEncoding.EncodeToString(pcmData),
	})
}

// NewStateMessage creates a state message
func NewStateMessage(connected bool, joints *JointState, headPose *HeadPoseState) (*Message, error) {
	return NewMessage(TypeState, StateData{
		Connected: connected,
		Joints:    joints,
		HeadPose:  headPose,
	})
}

// NewMotorMessage creates a motor command message
func NewMotorMessage(head HeadTarget, antennas [2]float64, bodyYaw float64) (*Message, error) {
	return NewMessage(TypeMotor, MotorCommand{
		Head:     head,
		Antennas: antennas,
		BodyYaw:  bodyYaw,
	})
}

// NewSpeakMessage creates a speak message with audio data
func NewSpeakMessage(audioData []byte, format string, sampleRate int) (*Message, error) {
	return NewMessage(TypeSpeak, SpeakData{
		Format:     format,
		SampleRate: sampleRate,
		Channels:   1,
		Data:       base64.StdEncoding.EncodeToString(audioData),
	})
}

// NewEmotionMessage creates an emotion command message
func NewEmotionMessage(name string, duration float64) (*Message, error) {
	return NewMessage(TypeEmotion, EmotionCommand{
		Name:     name,
		Duration: duration,
	})
}

// NewConfigMessage creates a configuration update message
func NewConfigMessage(camera *CameraConfig, audio *AudioConfig) (*Message, error) {
	return NewMessage(TypeConfig, ConfigUpdate{
		Camera: camera,
		Audio:  audio,
	})
}

// NewPingMessage creates a ping message
func NewPingMessage(id string) (*Message, error) {
	return NewMessage(TypePing, PingData{
		ID:        id,
		Timestamp: 0, // Will be set by NewMessage
	})
}

// NewPongMessage creates a pong response message
func NewPongMessage(id string, pingTS, pongTS int64) (*Message, error) {
	return NewMessage(TypePong, PongData{
		ID:        id,
		PingTS:    pingTS,
		PongTS:    pongTS,
		LatencyMs: pongTS - pingTS,
	})
}

// =============================================================================
// Helper functions for parsing messages
// =============================================================================

// GetFrameData extracts frame data from a message
func (m *Message) GetFrameData() (*FrameData, error) {
	var data FrameData
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// DecodeFrameData decodes the base64 image data
func (f *FrameData) DecodeFrameData() ([]byte, error) {
	return base64.StdEncoding.DecodeString(f.Data)
}

// GetDOAData extracts DOA data from a message
func (m *Message) GetDOAData() (*DOAData, error) {
	var data DOAData
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// GetMicData extracts mic data from a message
func (m *Message) GetMicData() (*MicData, error) {
	var data MicData
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// DecodeMicData decodes the base64 audio data
func (mic *MicData) DecodeMicData() ([]byte, error) {
	return base64.StdEncoding.DecodeString(mic.Data)
}

// GetStateData extracts state data from a message
func (m *Message) GetStateData() (*StateData, error) {
	var data StateData
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// GetMotorCommand extracts motor command from a message
func (m *Message) GetMotorCommand() (*MotorCommand, error) {
	var data MotorCommand
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// GetSpeakData extracts speak data from a message
func (m *Message) GetSpeakData() (*SpeakData, error) {
	var data SpeakData
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// DecodeSpeakData decodes the base64 audio data
func (s *SpeakData) DecodeSpeakData() ([]byte, error) {
	return base64.StdEncoding.DecodeString(s.Data)
}

// GetEmotionCommand extracts emotion command from a message
func (m *Message) GetEmotionCommand() (*EmotionCommand, error) {
	var data EmotionCommand
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// GetConfigUpdate extracts config update from a message
func (m *Message) GetConfigUpdate() (*ConfigUpdate, error) {
	var data ConfigUpdate
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// GetPingData extracts ping data from a message
func (m *Message) GetPingData() (*PingData, error) {
	var data PingData
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// GetPongData extracts pong data from a message
func (m *Message) GetPongData() (*PongData, error) {
	var data PongData
	if err := m.ParseData(&data); err != nil {
		return nil, err
	}
	return &data, nil
}



