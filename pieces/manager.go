/*
This is the heart of the downloader.

It tracks:

requested[piece][block]
received[piece][block]

It also implements a basic rarest-first scheduler
*/

package pieces

import (
	"crypto/sha1"
	"fmt"
	"sync"

	"torrent-client/torrent"
)

type Manager struct {
	mu sync.Mutex

	Torrent *torrent.MetaInfo

	Requested [][]bool

	Received [][]bool

	Availability []int

	PeerPieces map[string][]bool

	PieceData map[int][]byte
}

func NewManager(
	t *torrent.MetaInfo,
) *Manager {

	pieceCount := t.PieceCount()

	requested := make([][]bool, pieceCount)
	received := make([][]bool, pieceCount)

	for i := 0; i < pieceCount; i++ {

		blocks := BlocksForPiece(
			i,
			t.Info.PieceLength,
			t.Info.Length,
		)

		requested[i] = make(
			[]bool,
			len(blocks),
		)

		received[i] = make(
			[]bool,
			len(blocks),
		)
	}

	return &Manager{
		Torrent:      t,
		Requested:    requested,
		Received:     received,
		Availability: make([]int, pieceCount),
		PeerPieces:   make(map[string][]bool),
		PieceData:    make(map[int][]byte),
	}
}

func (m *Manager) SetPeerBitfield(
	peer string,
	bitfield []byte,
) {

	m.mu.Lock()
	defer m.mu.Unlock()

	pieceCount := m.Torrent.PieceCount()

	old := m.PeerPieces[peer]

	for i, has := range old {
		if has {
			m.Availability[i]--
		}
	}

	pieces := make(
		[]bool,
		pieceCount,
	)

	for i := 0; i < pieceCount; i++ {
		byteIndex := i / 8
		bitIndex := 7 - (i % 8)

		if byteIndex >= len(bitfield) {
			break
		}

		has := (bitfield[byteIndex]>>bitIndex)&1 == 1

		pieces[i] = has

		if has {
			m.Availability[i]++
		}
	}

	m.PeerPieces[peer] = pieces
}

func (m *Manager) AddPeerPiece(
	peer string,
	piece int,
) {

	m.mu.Lock()
	defer m.mu.Unlock()

	pieces := m.PeerPieces[peer]

	if pieces == nil {
		pieces = make(
			[]bool,
			m.Torrent.PieceCount(),
		)

		m.PeerPieces[peer] = pieces
	}

	if !pieces[piece] {
		pieces[piece] = true
		m.Availability[piece]++
	}
}

func (m *Manager) NextBlock(
	peer string,
) (*Block, bool) {

	m.mu.Lock()
	defer m.mu.Unlock()

	peerPieces := m.PeerPieces[peer]

	if peerPieces == nil {
		return nil, false
	}

	bestPiece := -1
	bestAvailability := int(^uint(0) >> 1)

	for piece := 0; piece < len(peerPieces); piece++ {

		if !peerPieces[piece] {
			continue
		}

		if m.pieceComplete(piece) {
			continue
		}

		if m.Availability[piece] < bestAvailability {
			bestPiece = piece
			bestAvailability = m.Availability[piece]
		}
	}

	if bestPiece == -1 {
		return nil, false
	}

	blocks := BlocksForPiece(
		bestPiece,
		m.Torrent.Info.PieceLength,
		m.Torrent.Info.Length,
	)

	for blockIndex, block := range blocks {

		if m.Requested[bestPiece][blockIndex] {
			continue
		}

		m.Requested[bestPiece][blockIndex] = true

		return &block, true
	}

	return nil, false
}

func (m *Manager) MarkReceived(
	block Block,
	data []byte,
) error {

	m.mu.Lock()
	defer m.mu.Unlock()

	if block.Piece < 0 ||
		block.Piece >= len(m.Received) {
		return fmt.Errorf("invalid piece index")
	}

	blockIndex := int(
		block.Begin / BlockSize,
	)

	if blockIndex >= len(m.Received[block.Piece]) {
		return fmt.Errorf("invalid block index")
	}

	m.Received[block.Piece][blockIndex] = true

	return nil
}

func (m *Manager) PieceComplete(
	piece int,
) bool {

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.pieceComplete(piece)
}

func (m *Manager) pieceComplete(
	piece int,
) bool {

	for _, received := range m.Received[piece] {
		if !received {
			return false
		}
	}

	return true
}

func (m *Manager) IsDone() bool {

	m.mu.Lock()
	defer m.mu.Unlock()

	for piece := range m.Received {

		for _, block := range m.Received[piece] {
			if !block {
				return false
			}
		}
	}

	return true
}

func (m *Manager) VerifyPiece(
	piece int,
	data []byte,
) bool {

	hash := sha1.Sum(data)

	expected := m.Torrent.PieceHash(piece)

	return hash == expected
}

func (m *Manager) StoreBlock(
	block Block,
	data []byte,
) bool {

	m.mu.Lock()
	defer m.mu.Unlock()

	pieceLength := m.Torrent.Info.PieceLength

	if block.Piece == m.Torrent.PieceCount()-1 {

		remaining := m.Torrent.Info.Length -
			int64(block.Piece)*pieceLength

		pieceLength = remaining
	}

	if m.PieceData[block.Piece] == nil {
		m.PieceData[block.Piece] =
			make([]byte, pieceLength)
	}

	copy(
		m.PieceData[block.Piece][block.Begin:],
		data,
	)

	blockIndex := int(
		block.Begin / BlockSize,
	)

	m.Received[block.Piece][blockIndex] = true

	for _, received := range m.Received[block.Piece] {
		if !received {
			return false
		}
	}

	return true
}

func (m *Manager) VerifyCompletedPiece(
	piece int,
) bool {

	m.mu.Lock()
	defer m.mu.Unlock()

	data := m.PieceData[piece]

	hash := sha1.Sum(data)

	expected := m.Torrent.PieceHash(piece)

	return hash == expected
}

func (m *Manager) ResetPeerRequests(
	peer string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// In this simplified implementation,
	// unreceived requested blocks are made
	// available again.

	for piece := range m.Requested {

		for block := range m.Requested[piece] {

			if !m.Received[piece][block] {
				m.Requested[piece][block] = false
			}
		}
	}
}