/*
4 bytes length
1 byte message ID
N bytes payload
*/

package peer

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	Choke         byte = 0
	Unchoke       byte = 1
	Interested    byte = 2
	NotInterested byte = 3
	Have          byte = 4
	Bitfield      byte = 5
	Request       byte = 6
	Piece         byte = 7
	Cancel        byte = 8
)

type Message struct {
	ID      byte
	Payload []byte
}

func ReadMessage(r io.Reader) (*Message, error) {
	var length uint32

	if err := binary.Read(
		r,
		binary.BigEndian,
		&length,
	); err != nil {
		return nil, err
	}

	if length == 0 {
		return nil, nil
	}

	if length > 1<<20 {
		return nil, fmt.Errorf(
			"message too large: %d",
			length,
		)
	}

	data := make([]byte, length)

	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return &Message{
		ID:      data[0],
		Payload: data[1:],
	}, nil
}

func WriteMessage(
	w io.Writer,
	id byte,
	payload []byte,
) error {

	length := uint32(1 + len(payload))

	var header [4]byte

	binary.BigEndian.PutUint32(
		header[:],
		length,
	)

	if _, err := w.Write(header[:]); err != nil {
		return err
	}

	if _, err := w.Write([]byte{id}); err != nil {
		return err
	}

	if len(payload) > 0 {
		_, err := w.Write(payload)
		return err
	}

	return nil
}

func WriteInterested(w io.Writer) error {
	return WriteMessage(
		w,
		Interested,
		nil,
	)
}

func WriteRequest(
	w io.Writer,
	index uint32,
	begin uint32,
	length uint32,
) error {

	payload := make([]byte, 12)

	binary.BigEndian.PutUint32(
		payload[0:4],
		index,
	)

	binary.BigEndian.PutUint32(
		payload[4:8],
		begin,
	)

	binary.BigEndian.PutUint32(
		payload[8:12],
		length,
	)

	return WriteMessage(
		w,
		Request,
		payload,
	)
}
