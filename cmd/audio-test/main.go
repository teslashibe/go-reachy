// Audio Test - Debug WebRTC audio capture with hraban/opus
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"gopkg.in/hraban/opus.v2"
)

const robotIP = "192.168.68.80"

var (
	packetsReceived int64
	bytesReceived   int64
	samplesDecoded  int64
	decodeErrors    int64
	mutex           sync.Mutex
	pcmBuffer       []int16
	recording       bool
)

func main() {
	fmt.Println("🎤 Audio Test v2 (hraban/opus)")
	fmt.Println("===============================")
	fmt.Println("Testing WebRTC audio capture with full libopus support\n")

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Connect to signalling server
	fmt.Println("Connecting to signalling server...")
	ws, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://%s:8443", robotIP), nil)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer ws.Close()

	// Get welcome
	var welcome struct {
		Type   string `json:"type"`
		PeerID string `json:"peerId"`
	}
	ws.ReadJSON(&welcome)
	fmt.Printf("Got peer ID: %s\n", welcome.PeerID[:8])

	// Get producer list
	ws.WriteJSON(map[string]string{"type": "list"})
	var listResp struct {
		Type      string `json:"type"`
		Producers []struct {
			ID   string            `json:"id"`
			Meta map[string]string `json:"meta"`
		} `json:"producers"`
	}
	ws.ReadJSON(&listResp)

	var producerID string
	for _, p := range listResp.Producers {
		if name, ok := p.Meta["name"]; ok && name == "reachymini" {
			producerID = p.ID
			break
		}
	}
	fmt.Printf("Found producer: %s\n", producerID[:8])

	// Create peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		fmt.Printf("❌ Failed to create peer connection: %v\n", err)
		os.Exit(1)
	}

	// Add audio transceiver
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		fmt.Printf("❌ Failed to add audio transceiver: %v\n", err)
		os.Exit(1)
	}

	// Create Opus decoder using hraban/opus (wraps libopus)
	// 48000 Hz, mono (we'll mix stereo to mono if needed)
	decoder, err := opus.NewDecoder(48000, 1)
	if err != nil {
		fmt.Printf("❌ Failed to create opus decoder: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Opus decoder created (48kHz mono)")

	// PCM buffer for decoded audio (max 120ms at 48kHz = 5760 samples)
	frameBuf := make([]int16, 5760)

	// Handle audio track
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("\n✅ Got track: %s (codec: %s)\n", track.Kind(), track.Codec().MimeType)

		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}

		fmt.Println("Starting audio decode loop...")

		for {
			rtpPacket, _, err := track.ReadRTP()
			if err != nil {
				fmt.Printf("❌ Read error: %v\n", err)
				return
			}

			packetsReceived++
			bytesReceived += int64(len(rtpPacket.Payload))

			// Decode Opus packet
			n, err := decoder.Decode(rtpPacket.Payload, frameBuf)
			if err != nil {
				decodeErrors++
				if decodeErrors <= 5 {
					fmt.Printf("⚠️  Decode error #%d: %v (payload: %d bytes, first bytes: %x)\n",
						decodeErrors, err, len(rtpPacket.Payload), rtpPacket.Payload[:min(8, len(rtpPacket.Payload))])
				}
				continue
			}

			samplesDecoded += int64(n)

			// Store samples if recording
			mutex.Lock()
			if recording {
				pcmBuffer = append(pcmBuffer, frameBuf[:n]...)
			}
			mutex.Unlock()

			// Log first few successful decodes
			if packetsReceived <= 3 {
				fmt.Printf("📦 Packet %d: %d bytes -> %d samples decoded\n",
					packetsReceived, len(rtpPacket.Payload), n)
			}
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Connection state: %s\n", state)
	})

	// Start session
	var sessionID string
	ws.WriteJSON(map[string]string{
		"type":   "startSession",
		"peerId": producerID,
	})

	// Handle signalling in goroutine
	go func() {
		for {
			var msg map[string]interface{}
			if err := ws.ReadJSON(&msg); err != nil {
				return
			}

			msgType, _ := msg["type"].(string)

			switch msgType {
			case "sessionStarted":
				sessionID, _ = msg["sessionId"].(string)
				fmt.Printf("Session started: %s\n", sessionID[:8])

			case "peer":
				if sdpData, ok := msg["sdp"].(map[string]interface{}); ok {
					sdpType, _ := sdpData["type"].(string)
					sdpStr, _ := sdpData["sdp"].(string)

					if sdpType == "offer" {
						offer := webrtc.SessionDescription{
							Type: webrtc.SDPTypeOffer,
							SDP:  sdpStr,
						}
						pc.SetRemoteDescription(offer)

						answer, _ := pc.CreateAnswer(nil)
						pc.SetLocalDescription(answer)

						ws.WriteJSON(map[string]interface{}{
							"type":      "peer",
							"sessionId": sessionID,
							"sdp": map[string]string{
								"type": answer.Type.String(),
								"sdp":  answer.SDP,
							},
						})
					}
				}

				if iceData, ok := msg["ice"].(map[string]interface{}); ok {
					candidate, _ := iceData["candidate"].(string)
					var sdpMid string
					if mid, ok := iceData["sdpMid"]; ok && mid != nil {
						sdpMid, _ = mid.(string)
					}
					var sdpMLineIndex uint16
					if idx, ok := iceData["sdpMLineIndex"]; ok && idx != nil {
						sdpMLineIndex = uint16(idx.(float64))
					}

					pc.AddICECandidate(webrtc.ICECandidateInit{
						Candidate:     candidate,
						SDPMid:        &sdpMid,
						SDPMLineIndex: &sdpMLineIndex,
					})
				}
			}
		}
	}()

	// ICE candidate handler
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil && sessionID != "" {
			init := candidate.ToJSON()
			ws.WriteJSON(map[string]interface{}{
				"type":      "peer",
				"sessionId": sessionID,
				"ice": map[string]interface{}{
					"candidate":     init.Candidate,
					"sdpMid":        init.SDPMid,
					"sdpMLineIndex": init.SDPMLineIndex,
				},
			})
		}
	})

	// Stats reporter and recording test
	go func() {
		time.Sleep(3 * time.Second)
		fmt.Println("\n📼 Starting 5 second recording test...")

		// Start recording
		mutex.Lock()
		pcmBuffer = nil
		recording = true
		mutex.Unlock()

		time.Sleep(5 * time.Second)

		// Stop recording
		mutex.Lock()
		recording = false
		samples := pcmBuffer
		mutex.Unlock()

		fmt.Printf("\n📼 Recorded %d samples (%.2f seconds at 48kHz)\n",
			len(samples), float64(len(samples))/48000.0)

		if len(samples) > 0 {
			// Save as WAV
			wavFile := "/tmp/audio_test.wav"
			err := writeWAV(wavFile, samples, 48000, 1)
			if err != nil {
				fmt.Printf("❌ Failed to write WAV: %v\n", err)
			} else {
				info, _ := os.Stat(wavFile)
				fmt.Printf("✅ Saved to %s (%d bytes)\n", wavFile, info.Size())
				fmt.Println("   You can play it with: afplay /tmp/audio_test.wav")
			}
		}
	}()

	// Stats ticker
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for range ticker.C {
			fmt.Printf("\r📊 Pkts: %d | Bytes: %d | Samples: %d | Errors: %d     ",
				packetsReceived, bytesReceived, samplesDecoded, decodeErrors)
		}
	}()

	fmt.Println("\nWaiting for audio... (will record 5s after 3s warmup)\n")

	<-sigChan
	fmt.Printf("\n\n📊 Final Stats:\n")
	fmt.Printf("   Packets received: %d\n", packetsReceived)
	fmt.Printf("   Bytes received: %d\n", bytesReceived)
	fmt.Printf("   Samples decoded: %d\n", samplesDecoded)
	fmt.Printf("   Decode errors: %d\n", decodeErrors)

	pc.Close()
}

func writeWAV(filename string, samples []int16, sampleRate, channels int) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	dataSize := len(samples) * 2
	fileSize := 36 + dataSize

	// RIFF header
	f.Write([]byte("RIFF"))
	binary.Write(f, binary.LittleEndian, uint32(fileSize))
	f.Write([]byte("WAVE"))

	// fmt chunk
	f.Write([]byte("fmt "))
	binary.Write(f, binary.LittleEndian, uint32(16))
	binary.Write(f, binary.LittleEndian, uint16(1))
	binary.Write(f, binary.LittleEndian, uint16(channels))
	binary.Write(f, binary.LittleEndian, uint32(sampleRate))
	binary.Write(f, binary.LittleEndian, uint32(sampleRate*channels*2))
	binary.Write(f, binary.LittleEndian, uint16(channels*2))
	binary.Write(f, binary.LittleEndian, uint16(16))

	// data chunk
	f.Write([]byte("data"))
	binary.Write(f, binary.LittleEndian, uint32(dataSize))

	for _, sample := range samples {
		binary.Write(f, binary.LittleEndian, sample)
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
