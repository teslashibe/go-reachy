// Package video provides WebRTC video streaming from Reachy Mini
package video

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

// Client connects to Reachy's WebRTC video stream via GStreamer signalling
type Client struct {
	robotIP       string
	signallingURL string

	ws      *websocket.Conn
	pc      *webrtc.PeerConnection
	wsMutex sync.Mutex

	myPeerID   string
	producerID string
	sessionID  string

	// H264 stream handling
	h264Buffer   []byte
	h264Mutex    sync.Mutex
	lastKeyframe []byte

	// Latest decoded frame
	latestFrame []byte
	frameMutex  sync.RWMutex
	frameReady  chan struct{}

	connected bool
	closed    bool
}

// NewClient creates a new WebRTC video client
func NewClient(robotIP string) *Client {
	return &Client{
		robotIP:       robotIP,
		signallingURL: fmt.Sprintf("ws://%s:8443", robotIP),
		frameReady:    make(chan struct{}, 1),
	}
}

// Connect establishes the WebRTC connection
func (c *Client) Connect() error {
	fmt.Println("  Connecting to signalling server...")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	var err error
	c.ws, _, err = dialer.Dial(c.signallingURL, nil)
	if err != nil {
		return fmt.Errorf("signalling connect failed: %w", err)
	}

	// Wait for welcome message
	fmt.Println("  Waiting for welcome...")
	if err := c.waitForWelcome(); err != nil {
		return fmt.Errorf("welcome failed: %w", err)
	}
	fmt.Printf("  Got peer ID: %s\n", c.myPeerID[:8])

	// Get producer list
	fmt.Println("  Finding producer...")
	if err := c.findProducer(); err != nil {
		return fmt.Errorf("find producer failed: %w", err)
	}
	fmt.Printf("  Found producer: %s\n", c.producerID[:8])

	// Create peer connection
	fmt.Println("  Creating peer connection...")
	if err := c.createPeerConnection(); err != nil {
		return fmt.Errorf("peer connection failed: %w", err)
	}

	// Start session
	fmt.Println("  Starting session...")
	if err := c.startSession(); err != nil {
		return fmt.Errorf("start session failed: %w", err)
	}

	// Start signalling handler
	go c.handleSignalling()

	// Wait for connection
	fmt.Println("  Waiting for video track...")
	select {
	case <-c.frameReady:
		fmt.Println("  ✅ Video connected!")
	case <-time.After(15 * time.Second):
		return fmt.Errorf("timeout waiting for video")
	}

	c.connected = true
	return nil
}

func (c *Client) waitForWelcome() error {
	c.ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, msg, err := c.ws.ReadMessage()
	c.ws.SetReadDeadline(time.Time{})

	if err != nil {
		return err
	}

	var welcome struct {
		Type   string `json:"type"`
		PeerID string `json:"peerId"`
	}
	if err := json.Unmarshal(msg, &welcome); err != nil {
		return err
	}
	if welcome.Type != "welcome" {
		return fmt.Errorf("expected welcome, got %s", welcome.Type)
	}
	c.myPeerID = welcome.PeerID
	return nil
}

func (c *Client) findProducer() error {
	c.wsMutex.Lock()
	err := c.ws.WriteJSON(map[string]string{"type": "list"})
	c.wsMutex.Unlock()
	if err != nil {
		return err
	}

	c.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := c.ws.ReadMessage()
	c.ws.SetReadDeadline(time.Time{})
	if err != nil {
		return err
	}

	var listResp struct {
		Type      string `json:"type"`
		Producers []struct {
			ID   string            `json:"id"`
			Meta map[string]string `json:"meta"`
		} `json:"producers"`
	}
	if err := json.Unmarshal(msg, &listResp); err != nil {
		return err
	}

	for _, p := range listResp.Producers {
		if name, ok := p.Meta["name"]; ok && name == "reachymini" {
			c.producerID = p.ID
			return nil
		}
	}
	return fmt.Errorf("reachymini producer not found in %d producers", len(listResp.Producers))
}

func (c *Client) createPeerConnection() error {
	config := webrtc.Configuration{}

	var err error
	c.pc, err = webrtc.NewPeerConnection(config)
	if err != nil {
		return err
	}

	// We want to receive video
	if _, err = c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return err
	}

	// Handle incoming video track
	c.pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("  Got track: %s (codec: %s)\n", track.Kind(), track.Codec().MimeType)
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			go c.handleVideoTrack(track)
		}
	})

	// Handle ICE candidates
	c.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			c.sendICECandidate(candidate)
		}
	})

	c.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("  Connection state: %s\n", state)
	})

	return nil
}

func (c *Client) startSession() error {
	c.wsMutex.Lock()
	err := c.ws.WriteJSON(map[string]string{
		"type":   "startSession",
		"peerId": c.producerID,
	})
	c.wsMutex.Unlock()
	return err
}

func (c *Client) handleSignalling() {
	for !c.closed {
		_, msg, err := c.ws.ReadMessage()
		if err != nil {
			if !c.closed {
				fmt.Printf("  Signalling error: %v\n", err)
			}
			return
		}

		var baseMsg struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionId"`
		}
		json.Unmarshal(msg, &baseMsg)

		switch baseMsg.Type {
		case "sessionStarted":
			c.sessionID = baseMsg.SessionID

		case "peer":
			c.handlePeerMessage(msg)

		case "endSession":
			return
		}
	}
}

func (c *Client) handlePeerMessage(msg []byte) {
	// Parse the peer message
	var peerMsg map[string]interface{}
	json.Unmarshal(msg, &peerMsg)

	// Check for SDP
	if sdpData, ok := peerMsg["sdp"]; ok {
		sdpMap := sdpData.(map[string]interface{})
		sdpType := sdpMap["type"].(string)
		sdpStr := sdpMap["sdp"].(string)

		if sdpType == "offer" {
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  sdpStr,
			}

			if err := c.pc.SetRemoteDescription(offer); err != nil {
				fmt.Printf("  SetRemoteDescription error: %v\n", err)
				return
			}

			answer, err := c.pc.CreateAnswer(nil)
			if err != nil {
				fmt.Printf("  CreateAnswer error: %v\n", err)
				return
			}

			if err := c.pc.SetLocalDescription(answer); err != nil {
				fmt.Printf("  SetLocalDescription error: %v\n", err)
				return
			}

			c.sendSDP(answer)
		}
	}

	// Check for ICE
	if iceData, ok := peerMsg["ice"]; ok {
		iceMap := iceData.(map[string]interface{})
		candidate := iceMap["candidate"].(string)

		var sdpMid string
		if mid, ok := iceMap["sdpMid"]; ok && mid != nil {
			sdpMid = mid.(string)
		}

		var sdpMLineIndex uint16
		if idx, ok := iceMap["sdpMLineIndex"]; ok && idx != nil {
			sdpMLineIndex = uint16(idx.(float64))
		}

		c.pc.AddICECandidate(webrtc.ICECandidateInit{
			Candidate:     candidate,
			SDPMid:        &sdpMid,
			SDPMLineIndex: &sdpMLineIndex,
		})
	}
}

func (c *Client) sendSDP(sdp webrtc.SessionDescription) {
	msg := map[string]interface{}{
		"type":      "peer",
		"sessionId": c.sessionID,
		"sdp": map[string]string{
			"type": sdp.Type.String(),
			"sdp":  sdp.SDP,
		},
	}
	c.wsMutex.Lock()
	c.ws.WriteJSON(msg)
	c.wsMutex.Unlock()
}

func (c *Client) sendICECandidate(candidate *webrtc.ICECandidate) {
	if c.sessionID == "" {
		return
	}

	init := candidate.ToJSON()
	msg := map[string]interface{}{
		"type":      "peer",
		"sessionId": c.sessionID,
		"ice": map[string]interface{}{
			"candidate":     init.Candidate,
			"sdpMid":        init.SDPMid,
			"sdpMLineIndex": init.SDPMLineIndex,
		},
	}
	c.wsMutex.Lock()
	c.ws.WriteJSON(msg)
	c.wsMutex.Unlock()
}

func (c *Client) handleVideoTrack(track *webrtc.TrackRemote) {
	// Signal that we got video
	select {
	case c.frameReady <- struct{}{}:
	default:
	}

	// H264 depacketizer to extract NAL units from RTP
	// NAL units need proper start codes (0x00 0x00 0x00 0x01)
	var h264Buffer bytes.Buffer
	var frameBuffer bytes.Buffer
	lastDecode := time.Now()
	frameCount := 0

	for !c.closed {
		// Read RTP packet
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			return
		}

		payload := rtpPacket.Payload
		if len(payload) < 2 {
			continue
		}

		// Parse H264 NAL unit header
		nalType := payload[0] & 0x1F

		switch {
		case nalType >= 1 && nalType <= 23:
			// Single NAL unit - add start code and NAL
			h264Buffer.Write([]byte{0x00, 0x00, 0x00, 0x01})
			h264Buffer.Write(payload)

		case nalType == 28: // FU-A (Fragmentation Unit)
			fuHeader := payload[1]
			startBit := (fuHeader & 0x80) != 0
			endBit := (fuHeader & 0x40) != 0
			nalType := fuHeader & 0x1F

			if startBit {
				// Start of fragmented NAL - add start code and reconstructed header
				h264Buffer.Write([]byte{0x00, 0x00, 0x00, 0x01})
				h264Buffer.WriteByte((payload[0] & 0xE0) | nalType)
			}
			// Add fragment payload (skip FU indicator and header)
			h264Buffer.Write(payload[2:])

			if endBit {
				// End of frame - decode
				frameBuffer.Write(h264Buffer.Bytes())
				h264Buffer.Reset()
			}

		case nalType == 24: // STAP-A (Single-time Aggregation Packet)
			// Skip STAP-A header byte
			offset := 1
			for offset < len(payload)-2 {
				nalSize := int(payload[offset])<<8 | int(payload[offset+1])
				offset += 2
				if offset+nalSize > len(payload) {
					break
				}
				h264Buffer.Write([]byte{0x00, 0x00, 0x00, 0x01})
				h264Buffer.Write(payload[offset : offset+nalSize])
				offset += nalSize
			}
		}

		// Decode accumulated NALs periodically
		if time.Since(lastDecode) > 100*time.Millisecond && h264Buffer.Len() > 1000 {
			c.decodeH264ToJPEG(h264Buffer.Bytes())
			frameCount++
			h264Buffer.Reset()
			lastDecode = time.Now()
		}
	}
}

func (c *Client) decodeH264ToJPEG(h264Data []byte) {
	if len(h264Data) < 500 {
		return
	}

	// Write H264 to temp file for debugging
	tmpH264 := "/tmp/reachy_video.h264"
	tmpJPEG := "/tmp/reachy_frame.jpg"

	// Append to file to accumulate a valid stream
	f, err := os.OpenFile(tmpH264, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	f.Write(h264Data)
	f.Close()

	// Get file size
	info, _ := os.Stat(tmpH264)
	if info == nil || info.Size() < 50000 {
		// Wait until we have enough data
		return
	}

	// Use ffmpeg to decode to JPEG
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "h264",
		"-i", tmpH264,
		"-vframes", "1",
		"-q:v", "2",
		tmpJPEG)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Keep accumulating if decode fails
		return
	}

	// Read the JPEG
	jpegData, err := os.ReadFile(tmpJPEG)
	if err == nil && len(jpegData) > 1000 {
		c.frameMutex.Lock()
		c.latestFrame = jpegData
		c.frameMutex.Unlock()

		// Clear the H264 buffer after successful decode
		os.Remove(tmpH264)
	}

	_ = output
}

// GetFrame returns the latest video frame as JPEG bytes
func (c *Client) GetFrame() ([]byte, error) {
	c.frameMutex.RLock()
	defer c.frameMutex.RUnlock()

	if c.latestFrame == nil {
		return nil, fmt.Errorf("no frame available")
	}

	frame := make([]byte, len(c.latestFrame))
	copy(frame, c.latestFrame)
	return frame, nil
}

// WaitForFrame waits for a frame to be available
func (c *Client) WaitForFrame(timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		frame, err := c.GetFrame()
		if err == nil {
			return frame, nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for frame")
}

// Close closes the WebRTC connection
func (c *Client) Close() {
	c.closed = true
	if c.pc != nil {
		c.pc.Close()
	}
	if c.ws != nil {
		c.ws.Close()
	}
}

// Unused import placeholder
var _ = rtp.Packet{}
var _ = http.Client{}
var _ = io.EOF
