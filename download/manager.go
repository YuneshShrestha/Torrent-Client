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
	Torrent *torrent.MetaInfo

	PeerID [20]byte

	Peers []tracker.Peer

	Pieces *pieces.Manager

	Storage *storage.Storage

	mu sync.Mutex
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

			conn.Choked = true

		case peer.Unchoke:

			conn.Choked = false

			if err := m.requestNext(
				conn,
				peerKey,
			); err != nil {
				return err
			}

		case peer.Bitfield:

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

	if len(payload) < 8 {
		return fmt.Errorf(
			"invalid piece message",
		)
	}

	pieceIndex := int(
		binary.BigEndian.Uint32(
			payload[0:4],
		),
	)

	begin := int64(
		binary.BigEndian.Uint32(
			payload[4:8],
		),
	)

	data := payload[8:]

	block := pieces.Block{
		Piece:  pieceIndex,
		Begin:  begin,
		Length: int64(len(data)),
	}

	complete := m.Pieces.StoreBlock(
		block,
		data,
	)

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
