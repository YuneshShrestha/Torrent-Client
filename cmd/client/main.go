package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
	"torrent-client/download"
	"torrent-client/torrent"
	"torrent-client/tracker"
)

func main() {

	torrentPath := flag.String(
		"torrent",
		"",
		"path to .torrent file",
	)

	output := flag.String(
		"output",
		"./downloads",
		"download directory",
	)

	port := flag.Int(
		"port",
		6881,
		"BitTorrent listening port",
	)

	flag.Parse()

	if *torrentPath == "" {
		log.Fatal(
			"usage: bittorrent -torrent file.torrent",
		)
	}

	t, err := torrent.Load(*torrentPath)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Torrent:")
	fmt.Println(" Name:", t.Info.Name)
	fmt.Println(" Size:", t.Info.Length)
	fmt.Println(" Pieces:", t.PieceCount())
	fmt.Println(
		" Piece length:",
		t.Info.PieceLength,
	)

	fmt.Printf(
		" Info hash: %x\n",
		t.InfoHash,
	)

	peerID := generatePeerID()

	fmt.Printf(
		" Peer ID: %x\n",
		peerID,
	)

	peers, err := announce(
		t,
		peerID,
		uint16(*port),
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(
		"Peers:",
		len(peers),
	)

	manager, err := download.New(
		t,
		peerID,
		*output,
	)

	if err != nil {
		log.Fatal(err)
	}

	manager.SetPeers(peers)

	start := time.Now()

	manager.Start()

	fmt.Println(
		"Finished in:",
		time.Since(start),
	)
}

func generatePeerID() [20]byte {

	var id [20]byte

	copy(
		id[:],
		"-GO0001-",
	)

	if _, err := rand.Read(id[8:]); err != nil {
		panic(err)
	}

	return id
}

func announce(
	t *torrent.MetaInfo,
	peerID [20]byte,
	port uint16,
) ([]tracker.Peer, error) {

	var trackers []string

	if t.Announce != "" {
		trackers = append(
			trackers,
			t.Announce,
		)
	}

	for _, tier := range t.AnnounceList {
		trackers = append(
			trackers,
			tier...,
		)
	}

	var allPeers []tracker.Peer

	for _, trackerURL := range trackers {

		fmt.Println(
			"Announcing to:",
			trackerURL,
		)

		var (
			peers []tracker.Peer
			err   error
		)

		switch {
		case strings.HasPrefix(
			trackerURL,
			"http://",
		) || strings.HasPrefix(
			trackerURL,
			"https://",
		):

			peers, err = tracker.AnnounceHTTP(
				trackerURL,
				t.InfoHash,
				peerID,
				port,
				t.Info.Length,
			)

		case strings.HasPrefix(
			trackerURL,
			"udp://",
		):
			peers, err = tracker.AnnounceUDP(
				trackerURL,
				t.InfoHash,
				peerID,
				port,
				t.Info.Length,
			)

		default:
			continue
		}

		if err != nil {
			fmt.Println(
				"Tracker error:",
				err,
			)

			continue
		}

		allPeers = append(
			allPeers,
			peers...,
		)
	}

	if len(allPeers) == 0 {
		return nil, fmt.Errorf(
			"no peers found",
		)
	}

	return uniquePeers(allPeers), nil
}

func uniquePeers(
	input []tracker.Peer,
) []tracker.Peer {

	seen := make(
		map[string]bool,
	)

	var result []tracker.Peer

	for _, p := range input {

		key := fmt.Sprintf(
			"%s:%d",
			p.IP,
			p.Port,
		)

		if seen[key] {
			continue
		}

		seen[key] = true

		result = append(
			result,
			p,
		)
	}

	return result
}
