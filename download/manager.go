// This is where everything comes together.

package download

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"
	"torrent-client/peer"
	"torrent-client/pieces"
	"torrent-client/storage"
	"torrent-client/torrent"
	"torrent-client/tracker"
)

type Manager struct {
	Torrent *torrent.MetaInfo ///  What are we downloading?

	PeerID [20]byte ///  Who are we?

	Peers []tracker.Peer ///  Who can we download from?

	Pieces *pieces.Manager ///  Which pieces/blocks do we need?

	Storage *storage.Storage /// Where do we save the data?

	mu sync.Mutex /// Protect shared data
}

func New(
	t *torrent.MetaInfo,
	peerID [20]byte,
	output string,
) (*Manager, error) {

	store, err := storage.New(
		output,
		&t.Info,
	)
	if err != nil {
		return nil, err
	}

	/// Note: Peers is initially empty because you haven't contacted the tracker yet.
	return &Manager{
		Torrent: t,
		PeerID:  peerID,
		Pieces:  pieces.NewManager(t),
		Storage: store,
	}, nil
}

func (m *Manager) SetPeers(
	peers []tracker.Peer,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Peers = peers
}

func (m *Manager) Start() {

	var wg sync.WaitGroup

	maxPeers := 50

	if len(m.Peers) < maxPeers {
		maxPeers = len(m.Peers)
	}

	for i := 0; i < maxPeers; i++ {

		p := m.Peers[i]

		wg.Add(1)

		go func() {
			defer wg.Done()

			m.peerLoop(p)
		}()
	}

	wg.Wait()
}

func (m *Manager) peerLoop(
	p tracker.Peer,
) {

	address := fmt.Sprintf(
		"%s:%d",
		p.IP,
		p.Port,
	)

	log.Printf(
		"connecting to %s",
		address,
	)

	for attempt := 0; attempt < 5; attempt++ {

		if m.Pieces.IsDone() {
			return
		}

		err := m.downloadFromPeer(address)

		if err == nil {
			return
		}

		m.Pieces.ResetPeerRequests(address)

		log.Printf(
			"peer %s failed: %v",
			address,
			err,
		)

		time.Sleep(
			time.Duration(attempt+1) *
				time.Second,
		)
	}
}

func (m *Manager) downloadFromPeer(
	address string,
) error {
	/*
		Connects to the peer and performs the BitTorrent handshake.

		Your client
			|
			| handshake
			↓
		Peer
	*/
	conn, err := peer.Connect(
		address,
		m.Torrent.InfoHash,
		m.PeerID,
	)

	if err != nil {
		return err
	}

	defer conn.Close()

	peerKey := address

	for {
		if m.Pieces.IsDone() {
			return nil
		}

		msg, err := peer.ReadMessage(
			conn.Conn,
		)

		if err != nil {
			return err
		}

		if msg == nil {
			continue
		}

		conn.LastActivity = time.Now()

		switch msg.ID {

		case peer.Choke:
			/// I'm not allowing you to request data right now.
			conn.Choked = true

		case peer.Unchoke:
			/// You can request data now.
			conn.Choked = false

			/*
				Unchoke
				|
				requestNext()
				|
				Find needed block
				|
				Send REQUEST
			*/
			if err := m.requestNext(
				conn,
				peerKey,
			); err != nil {
				return err
			}

		case peer.Bitfield:
			/*
				The peer tells us which pieces it has.

				Peer bitfield:
				Piece:  0 1 2 3 4 5
						↓ ↓ ↓ ↓ ↓ ↓
						1 0 1 1 0 1
			*/
			conn.Bitfield = msg.Payload

			m.Pieces.SetPeerBitfield(
				peerKey,
				msg.Payload,
			)

			if !conn.Choked {
				if err := m.requestNext(
					conn,
					peerKey,
				); err != nil {
					return err
				}
			}

		case peer.Have:
			/// Tells us about one newly available piece
			///
			/*
				WHY requestNext()?

				The peer has just announced that a new piece is available.

				If we are unchoked, we can immediately reconsider
				what block we should request from this peer.
			*/
			if len(msg.Payload) != 4 {
				continue
			}

			pieceIndex := int(
				binary.BigEndian.Uint32(
					msg.Payload,
				),
			)

			if pieceIndex >= 0 &&
				pieceIndex < m.Torrent.PieceCount() {

				m.Pieces.AddPeerPiece(
					peerKey,
					pieceIndex,
				)
			}

			if !conn.Choked {
				if err := m.requestNext(
					conn,
					peerKey,
				); err != nil {
					return err
				}
			}

		case peer.Piece:

			if err := m.handlePiece(
				conn,
				peerKey,
				msg.Payload,
			); err != nil {
				return err
			}
		}
	}
}

func (m *Manager) requestNext(
	conn *peer.Connection,
	peerKey string,
) error {

	if conn.Choked {
		return nil
	}

	/// Find the rarest available piece
	block, ok := m.Pieces.NextBlock(
		peerKey,
	)

	if !ok {
		return nil
	}

	return peer.WriteRequest(
		conn.Conn,
		uint32(block.Piece),
		uint32(block.Begin),
		uint32(block.Length),
	)
}

func (m *Manager) handlePiece(
	conn *peer.Connection,
	peerKey string,
	payload []byte,
) error {
	/*
		PIECE message
		┌─────────────┬─────────────┬───────────────┐
		│ pieceIndex  │    begin    │     data      │
		│   4 bytes   │   4 bytes   │   actual data │
		└─────────────┴─────────────┴───────────────┘

		Eg:
		Piece Index = 2
		Begin = 16384
		data = 16 KB

		This means the peer has piece 2 and it has 16 KB of data which
		starts at byte 16384.
	*/

	if len(payload) < 8 {
		return fmt.Errorf(
			"invalid piece message",
		)
	}

	/// Which piece it belongs to
	/// Eg: 2 means this data belongs to piece 2
	pieceIndex := int(
		binary.BigEndian.Uint32(
			payload[0:4],
		),
	)

	/// Where the data starts
	/// Eg: 00 00 40 00
	/// Means the data starts at byte 16384
	/// 0x40 = 64 and 64 * 256^1 = 16384
	/*
		Byte 1    Byte 2    Byte 3    Byte 4
		00        00        40        00
		↓         ↓         ↓         ↓
		256³      256²      256¹      256⁰
	*/
	begin := int64(
		binary.BigEndian.Uint32(
			payload[4:8],
		),
	)

	/// The actual data
	data := payload[8:]

	/// Store the block
	block := pieces.Block{
		Piece:  pieceIndex,
		Begin:  begin,
		Length: int64(len(data)),
	}

	/*
		StoreBlock() has 2 jobs:

		1. Save the actual data in memort
		2. Remeber that this block has been received
	*/
	complete := m.Pieces.StoreBlock(
		block,
		data,
	)

	/// Calculate the global offset
	/*
		Example:
		pieceIndex  = 2
		pieceLength = 32768
		begin       = 16384

		Therefore:

		2 × 32768 + 16384
		= 65536 + 16384
		= 81920

		So the global offset is 81920

	*/
	globalOffset :=
		int64(pieceIndex)*
			m.Torrent.Info.PieceLength +
			begin

	if err := m.Storage.WriteAt(
		data,
		globalOffset,
	); err != nil {
		return err
	}

	if complete {

		if !m.Pieces.VerifyCompletedPiece(
			pieceIndex,
		) {
			log.Printf(
				"piece %d failed SHA-1 verification",
				pieceIndex,
			)

			return fmt.Errorf(
				"piece %d hash mismatch",
				pieceIndex,
			)
		}

		log.Printf(
			"piece %d complete",
			pieceIndex,
		)
	}

	if m.Pieces.IsDone() {
		log.Println("DOWNLOAD COMPLETE")
		return nil
	}

	return m.requestNext(
		conn,
		peerKey,
	)
}
