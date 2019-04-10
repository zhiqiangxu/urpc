package urpc

// MediaType for media type
type MediaType int

const (
	// MTAudio for audio
	MTAudio MediaType = iota
	// MTVideo for video
	MTVideo
)

// Cmd for urpc
type Cmd uint8

// Packet for urpc
type Packet struct {
	Cmd     Cmd
	Payload []byte
}
