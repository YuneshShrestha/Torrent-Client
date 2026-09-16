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
	"math"
	"sync"

	"torrent-client/torrent"
)

type Manager struct {
	mu sync.Mutex

	Torrent *torrent.MetaInfo

	/// Did we request this block?
	Requested [][]bool

	/// Did we receive this block?
	Received [][]bool

	/// How many peers have each piece?
	/*
		    Piece 0 -> 3 peers have it
			Piece 1 -> 1 peer has it
			Piece 2 -> 5 peers have it
			Piece 3 -> 0 peers have it
	*/
	Availability []int

	/// Which pieces does each peer have?
	/*
		Imagine we have 4 pieces and 3 peers:

		Peer A → [true, true, false, false]
		Peer B → [false, true, true, false]
		Peer C → [true, false, true, true]
	*/
	PeerPieces map[string][]bool

	/// Downloaded piece data
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

/*
SetPeerBitfield updates what pieces a specific peer has.

WHY:
A BitTorrent peer sends a "bitfield" message to tell us which
pieces of the torrent it already has.

Example:

Torrent has 10 pieces:

Piece:      0 1 2 3 4 5 6 7 8 9
Bitfield:   1 0 1 1 0 1 1 0 1 1

This means the peer has:

Piece 0 ✓
Piece 1 ✗
Piece 2 ✓
Piece 3 ✓
Piece 4 ✗
Piece 5 ✓
Piece 6 ✓
Piece 7 ✗
Piece 8 ✓
Piece 9 ✓

The bitfield is compact because 8 pieces are stored inside 1 byte.

Example:

10110110 11000000
└───────┘ └───────┘
byte 0    byte 1

IMPORTANT:
A byte contains 8 bits.

For piece index i:

byteIndex = i / 8
bitIndex  = 7 - (i % 8)

Why?

Pieces 0-7 are stored in byte 0:

Piece 0 → bit 7
Piece 1 → bit 6
Piece 2 → bit 5
Piece 3 → bit 4
Piece 4 → bit 3
Piece 5 → bit 2
Piece 6 → bit 1
Piece 7 → bit 0

Pieces 8-15 are stored in byte 1:

Piece 8  → bit 7
Piece 9  → bit 6
...

/8 tells us WHICH BYTE contains the piece.

Example:

0 / 8 = 0
7 / 8 = 0
8 / 8 = 1
9 / 8 = 1

%8 tells us the position INSIDE that group of 8.

7 - (i % 8) is used because BitTorrent reads the most
significant bit first:

10110110
76543210
↑      ↑
bit 7  bit 0

HOW THE BIT IS READ:

has := (bitfield[byteIndex] >> bitIndex) & 1 == 1

Example for Piece 2:

byteIndex = 2 / 8 = 0
bitIndex  = 7 - (2 % 8) = 5

bitfield[0] = 10110110

		↑
	bit 5

Shift right by 5:

10110110 >> 5 = 00000101

Then & 1:

00000101
&
00000001
----------
00000001 = 1

Therefore:

has = true

So:

pieces[2] = true

AVAILABILITY:

m.Availability[i] tells us how many peers have piece i.

Example:

Peer A → pieces 0, 2, 3
Peer B → pieces 0, 1, 3
Peer C → pieces 1, 3, 5

Then:

Piece 0 → 2 peers
Piece 1 → 2 peers
Piece 2 → 1 peer
Piece 3 → 3 peers
Piece 5 → 1 peer

This is later used by NextBlock() for the
RAREST-FIRST strategy.

RAREST-FIRST means:

Prefer a piece that is available from fewer peers.

WHY REMOVE OLD AVAILABILITY FIRST:

The peer may already exist in m.PeerPieces.

Suppose old information says:

Peer A has: 0, 2

And:

Availability[0] = 3
Availability[2] = 2

Before replacing Peer A's information, remove its old contribution:

Availability[0]--  → 3 becomes 2
Availability[2]--  → 2 becomes 1

Then decode the new bitfield and add the peer's new pieces.

Otherwise, we could count the same peer twice.

Example:

Old: Peer A has Piece 2
New: Peer A still has Piece 2

Wrong:

2 → 3

Correct:

Remove old: 2 → 1
Add new:    1 → 2

OVERALL FLOW:

Peer sends Bitfield

	↓

"These are the pieces I have"

	↓

SetPeerBitfield()

	↓

Remove peer's OLD piece information

	↓

Decode each bit from the bitfield

	↓

Store result in []bool

	↓

Update Availability[]

	↓

Save:

	m.PeerPieces[peer] = pieces
		↓

# NextBlock() can choose the rarest piece

IN SHORT:

Bitfield []byte

	↓

Decode

	↓

[]bool of pieces

	↓

Update Availability

	↓

Rarest-first downloading
*/
func (m *Manager) SetPeerBitfield(
	peer string,
	bitfield []byte,
) {
	/*
		Your manager contains shared data:

		m.PeerPieces
		m.Availability

		Multiple peer goroutines might modify these at the same time. So we lock.
	*/
	m.mu.Lock()
	defer m.mu.Unlock()

	/// Get the number of pieces
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

	/// Find the rarest available piece
	///
	/// bestPiece starts as -1 as -1 can safely mean "nothing selected."
	/// bestAvailability starts as the largest possible int because you are looking for the smallest availability..

	/*
		Initial:
		bestAvailability = MAX_INT

		Piece 0 → availability = 5
		5 < MAX_INT → YES
		bestAvailability = 5

		Piece 1 → availability = 3
		3 < 5 → YES
		bestAvailability = 3

		Piece 2 → availability = 7
		7 < 3 → NO

		Piece 3 → availability = 2
		2 < 3 → YES
		bestAvailability = 2

		So piece 3 is the rarest piece among the pieces this peer has.
	*/
	bestPiece := -1
	bestAvailability := math.MaxInt

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
	/// checks every piece. If any piece is incomplete, it returns false
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

func (m *Manager) StoreBlock(
	block Block,
	data []byte,
) bool {

	m.mu.Lock()
	defer m.mu.Unlock()

	/// Get the normal piece size [Usually, every piece except the last one has this size.]
	pieceLength := m.Torrent.Info.PieceLength

	/// Check if this is the last piece
	/// if it is then calculate the last piece's actual size
	if block.Piece == m.Torrent.PieceCount()-1 {
		/*
			Ex:

			Full Torrent Size = 50000
			And for the last piece: Piece #3 [There are 4 pieces in this torrent 0-3]

			50000 - (3 * 16384) = 848

			So the last piece's size is 848
		*/
		remaining := m.Torrent.Info.Length -
			int64(block.Piece)*pieceLength

		pieceLength = remaining
	}

	/// Create storage for the piece
	if m.PieceData[block.Piece] == nil {
		m.PieceData[block.Piece] =
			make([]byte, pieceLength)
	}

	/// Put the received block into the piece
	copy(
		m.PieceData[block.Piece][block.Begin:],
		data,
	)

	/*
		Suppose:

		BlockSize = 16384
		and:
		begin = 16384

		Then:
		16384 / 16384
		= 1

		So blockIndex = 1

		Meaning:

		Piece #2
		Block 0 → begin 0
		Block 1 → begin 16384
		Block 2 → begin 32768
	*/
	blockIndex := int(
		block.Begin / BlockSize,
	)

	/*
		This is used to check for particular pieces which blocks are
		received.

		Received[2]:
		Block 0 → true
		Block 1 → true
		Block 2 → false
	*/
	m.Received[block.Piece][blockIndex] = true

	//// Check every block for this piece to see if they are all received
	for _, received := range m.Received[block.Piece] {
		if !received {
			return false
		}
	}

	return true
}

/*
Downloaded piece

	   │
	   ▼
	SHA-1
	   │
	   ▼

calculated hash

	│
	│ compare
	▼

expected torrent hash
*/
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

/*
Requested = true
Received  = false

We asked for it but haven't received it.

This is the important case.

Maybe:

peer disconnected
peer stopped responding
request timed out
connection failed

So we need to allow another peer to request it. So we reset it.
*/
func (m *Manager) ResetPeerRequests(
	peer string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for piece := range m.Requested {

		for block := range m.Requested[piece] {

			if !m.Received[piece][block] {
				m.Requested[piece][block] = false
			}
		}
	}
}
