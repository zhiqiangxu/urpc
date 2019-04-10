package urpc

import (
	"errors"

	"github.com/golang/protobuf/proto"
)

var (
	errInvalidPacketSize = errors.New("invalid packet size")
)

func decodePacket(bytes []byte) (p Packet, err error) {
	rawPacket := RawPacket{}

	err = proto.Unmarshal(bytes, rawPacket)
	if err != nil {
		return
	}

	if rawPacket.PayloadSize != len(rawPacket.Payload) {
		err = errInvalidPacketSize
		return
	}

	p.Cmd = Cmd(rawPacket.Cmd)
	p.Payload = rawPacket.Payload
	return
}

func encodePacket(cmd Cmd, payload []byte) (bytes []byte, err error) {
	rawPacket := RawPacket{Cmd: cmd, PayloadSize: len(payload), Payload: payload}

	bytes, err = proto.Marshal(rawPacket)
	return
}
