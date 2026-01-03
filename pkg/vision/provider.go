package vision

// Provider interface for camera access.
type Provider interface {
	CaptureFrame() ([]byte, error) // Returns JPEG image data
}


