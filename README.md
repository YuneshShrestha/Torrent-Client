# BitTorrent Client in Go

A BitTorrent client written in **Go** for learning how peer-to-peer file sharing works internally.

The project implements the core BitTorrent download flow:

* `.torrent` file parsing
* Bencode decoding and encoding
* HTTP and UDP tracker communication
* Peer discovery
* TCP peer connections
* BitTorrent handshake
* Bitfield and `HAVE` messages
* Choke / unchoke handling
* Piece and block management
* Requesting blocks from multiple peers
* Writing downloaded data to disk
* SHA-1 piece verification
* Basic rarest-first piece selection
* Single-file and multi-file torrent support
* Peer reconnection

The project is intentionally built from the protocol level rather than using an existing BitTorrent library.

---

## Architecture

```text
                     ┌─────────────────────┐
                     │     CLI / main.go   │
                     │   User Entry Point  │
                     └──────────┬──────────┘
                                │
                                ▼
                     ┌─────────────────────┐
                     │   Torrent Metadata  │
                     │ torrent/metainfo.go  │
                     └──────────┬──────────┘
                                │
                                ▼
                     ┌─────────────────────┐
                     │      Trackers       │
                     │ HTTP / UDP Tracker  │
                     └──────────┬──────────┘
                                │
                                │ Peer Addresses
                                ▼
              ┌────────────────────────────────────┐
              │             Peer Layer              │
              │ Handshake + Messages + Connection  │
              └────────────────┬───────────────────┘
                               │
                               │ Pieces / Blocks
                               ▼
              ┌────────────────────────────────────┐
              │          Piece Manager              │
              │ Requested / Received / Scheduling  │
              └────────────────┬───────────────────┘
                               │
                               │ Completed blocks
                               ▼
                     ┌─────────────────────┐
                     │       Storage       │
                     │      WriteAt()      │
                     └─────────────────────┘
```

---

# Complete Torrent Download Flow

```text
                  User
                   │
                   ▼
             ┌───────────────┐
             │ .torrent file │
             └───────┬───────┘
                     │
                     ▼
             ┌───────────────┐
             │ Decode Bencode│
             └───────┬───────┘
                     │
                     ▼
          ┌──────────────────────┐
          │ Read torrent metadata │
          │                      │
          │ announce             │
          │ info                 │
          │ piece length         │
          │ pieces               │
          │ file information     │
          └──────────┬───────────┘
                     │
                     ▼
              Calculate SHA-1
             of encoded "info"
                     │
                     ▼
             ┌───────────────┐
             │   info_hash   │
             └───────┬───────┘
                     │
                     ▼
             ┌───────────────┐
             │    Tracker    │
             └───────┬───────┘
                     │
                     │ Peer IP + Port
                     ▼
             ┌─────────────────┐
             │  Peer Manager   │
             └────────┬────────┘
                      │
             ┌────────┼────────┐
             ▼        ▼        ▼
          Peer A    Peer B    Peer C
             │        │        │
             ▼        ▼        ▼
         Handshake Handshake Handshake
             │        │        │
             ▼        ▼        ▼
          Bitfield  Bitfield  HAVE
             │        │        │
             └────────┼────────┘
                      ▼
                Select Pieces
                      │
                      ▼
                  Split into
                    Blocks
                      │
                      ▼
             ┌──────────────────┐
             │  Request Blocks  │
             └────────┬─────────┘
                      │
                      ▼
                Receive PIECE
                  messages
                      │
                      ▼
                Store block data
                      │
                      ▼
                Piece completed?
                   /       \
                 No         Yes
                 │           │
                 │           ▼
                 │       SHA-1 verify
                 │           │
                 │       ┌───┴───┐
                 │       │       │
                 │      FAIL    PASS
                 │       │       │
                 │       ▼       ▼
                 │     Retry    Mark
                 │             complete
                 │                 │
                 └─────────────────┤
                                   ▼
                             More pieces?
                               /      \
                             Yes       No
                              │         │
                              ▼         ▼
                         Continue    Download
                                    Complete
```

---

# What Is a Torrent File?

A `.torrent` file is a **metadata file**.

It does **not** contain the actual movie, Linux ISO, game, or other downloaded content.

Instead, it tells the client:

```text
Where can I find peers?
What files am I downloading?
How large are the pieces?
What should every piece's SHA-1 hash be?
```

A simplified torrent looks like:

```text
.torrent
   │
   ├── announce
   │     └── tracker URL
   │
   └── info
         ├── name
         ├── piece length
         ├── length / files
         └── pieces
               ├── SHA-1 piece hash
               ├── SHA-1 piece hash
               ├── SHA-1 piece hash
               └── ...
```

---

# Project Structure

```text
bittorrent/
│
├── go.mod
│
├── cmd/
│   └── client/
│       └── main.go
│
├── torrent/
│   ├── bencode.go
│   ├── metainfo.go
│   └── files.go
│
├── tracker/
│   ├── http.go
│   └── udp.go
│
├── peer/
│   ├── handshake.go
│   ├── message.go
│   └── connection.go
│
├── pieces/
│   ├── block.go
│   └── manager.go
│
├── storage/
│   └── storage.go
│
└── download/
    └── manager.go
```

Each package has a specific responsibility.

---

# `cmd/client/main.go`

This is the **entry point** of the application.

It handles the command-line interface.

Example:

```bash
./bittorrent \
    -torrent ubuntu.torrent \
    -output ./downloads
```

The main function connects all the components together.

### Main Responsibilities

```text
CLI arguments
     │
     ▼
Load torrent
     │
     ▼
Parse metadata
     │
     ▼
Create peer ID
     │
     ▼
Contact tracker
     │
     ▼
Create download manager
     │
     ▼
Start downloading
```

It is essentially the **orchestrator** of the entire client.

---

# `torrent/bencode.go`

This file implements **Bencode**.

Bencode is the data encoding format used by BitTorrent.

It supports four basic types:

```text
String
Integer
List
Dictionary
```

## String

```text
4:spam
```

Means:

```text
"spam"
```

## Integer

```text
i42e
```

Means:

```text
42
```

## List

```text
l4:spam4:eggse
```

Means approximately:

```text
["spam", "eggs"]
```

## Dictionary

```text
d3:cow3:moo4:spam4:eggse
```

Means approximately:

```text
{
    "cow": "moo",
    "spam": "eggs"
}
```

The decoder converts raw torrent bytes into Go values.

```text
.torrent bytes
      │
      ▼
 Bencode Decoder
      │
      ▼
Go values
(map / string / int / []interface{})
```

The encoder is also important because the BitTorrent `info_hash` is calculated from the **exact Bencoded `info` dictionary**.

---

# `torrent/metainfo.go`

This file converts the decoded Bencode data into a structured torrent object.

For example:

```text
Torrent
│
├── Announce
│
├── Info
│   ├── Name
│   ├── PieceLength
│   ├── Length
│   ├── Files
│   └── Pieces
│
└── InfoHash
```

One of the most important values is:

```text
info_hash
```

The `info_hash` identifies the torrent.

It is calculated as:

```text
SHA1(
    Bencoded info dictionary
)
```

This value is sent to trackers and peers.

---

# `torrent/files.go`

This file handles torrent file information.

BitTorrent supports both single-file and multi-file torrents.

## Single-file Torrent

```text
ubuntu.iso
```

or:

```text
movie.mkv
```

## Multi-file Torrent

```text
Ubuntu/
├── file1.iso
├── file2.txt
├── folder/
│   ├── file3
│   └── file4
```

`files.go` calculates the total torrent size and provides information about where downloaded bytes should be written.

---

# `tracker/http.go`

This file communicates with **HTTP trackers**.

The tracker helps the client discover peers.

The client sends information such as:

```text
info_hash
peer_id
port
uploaded
downloaded
left
```

Conceptually:

```text
Client
  │
  │ announce
  ▼
HTTP Tracker
  │
  │ list of peers
  ▼
Client
```

The tracker responds with peers:

```text
Peer 1 → 192.168.1.10:6881
Peer 2 → 192.168.1.20:51432
Peer 3 → 10.0.0.5:6881
```

The client can then connect to those peers.

---

# `tracker/udp.go`

Some trackers use the **UDP tracker protocol** instead of HTTP.

The basic flow is:

```text
Client
  │
  │ UDP Connect
  ▼
Tracker
  │
  │ Connection ID
  ▼
Client
  │
  │ Announce
  ▼
Tracker
  │
  │ Peers
  ▼
Client
```

UDP trackers avoid the overhead of HTTP.

The package handles the binary UDP tracker messages and extracts peer addresses.

---

# `peer/handshake.go`

Once we discover a peer, we need to establish a BitTorrent connection.

The first important message is the **handshake**.

The standard handshake is:

```text
┌───────────┬──────────────┬───────────┬────────────┬────────────┐
│ pstrlen   │ pstr         │ reserved  │ info_hash  │ peer_id    │
│ 1 byte    │ 19 bytes     │ 8 bytes   │ 20 bytes   │ 20 bytes   │
└───────────┴──────────────┴───────────┴────────────┴────────────┘
```

Total:

```text
1 + 19 + 8 + 20 + 20 = 68 bytes
```

The handshake tells the peer:

```text
I am speaking BitTorrent.
I want this torrent.
Here is my peer ID.
```

Flow:

```text
Client                         Peer
  │                              │
  │──── TCP connection ─────────>│
  │                              │
  │──── Handshake ──────────────>│
  │                              │
  │<──── Handshake ──────────────│
  │                              │
  │       Connection ready       │
```

---

# `peer/message.go`

After the handshake, peers communicate using BitTorrent messages.

A normal message looks like:

```text
┌───────────────┬─────────┬─────────────┐
│ Length Prefix │ Message │   Payload   │
│   4 bytes     │ 1 byte  │   N bytes   │
└───────────────┴─────────┴─────────────┘
```

Important message IDs:

```text
0 → choke
1 → unchoke
2 → interested
3 → not interested
4 → have
5 → bitfield
6 → request
7 → piece
8 → cancel
```

For example, a `REQUEST` contains:

```text
index
begin
length
```

Example:

```text
Piece index = 10
Offset       = 16384
Length       = 16384
```

A `PIECE` message contains the requested data.

---

# `peer/connection.go`

This file manages the actual connection to a peer.

Responsibilities:

```text
TCP connection
     │
     ▼
Handshake
     │
     ▼
Read messages
     │
     ├── BITFIELD
     ├── HAVE
     ├── CHOKE
     ├── UNCHOKE
     └── PIECE
```

It also handles:

* Sending `INTERESTED`
* Sending block requests
* Reading blocks
* Detecting peer disconnects
* Reconnecting
* Processing peer state

---

# Peer State

A peer can be:

```text
CHOKED
   │
   │ UNCHOKE
   ▼
UNCHOKED
```

A client generally cannot request data while it is choked.

The normal flow is:

```text
Connect
   │
   ▼
Handshake
   │
   ▼
Receive Bitfield
   │
   ▼
Send INTERESTED
   │
   ▼
Wait for UNCHOKE
   │
   ▼
Request blocks
```

---

# `pieces/block.go`

A torrent is divided into **pieces**.

For example:

```text
Torrent size = 100 MB
Piece size   = 1 MB
```

Approximately:

```text
100 pieces
```

A piece is usually too large to request as one network message.

Therefore, pieces are divided into **blocks**.

Example:

```text
Piece #10
│
├── Block 0 → 16 KB
├── Block 1 → 16 KB
├── Block 2 → 16 KB
├── Block 3 → 16 KB
└── ...
```

A block is represented by:

```text
index
begin
length
```

Example:

```text
index  = 10
begin  = 32768
length = 16384
```

---

# `pieces/manager.go`

This is one of the most important components.

The Piece Manager answers:

```text
Which piece should I download?
Which blocks are available?
Which blocks have already been requested?
Which blocks have been received?
Is a piece complete?
Is the piece valid?
```

A piece can have states such as:

```text
Not requested
     │
     ▼
Requested
     │
     ▼
Received
     │
     ▼
Verified
```

Conceptually:

```text
Piece 0
├── Block 0 → received
├── Block 1 → received
├── Block 2 → requested
└── Block 3 → not requested

Piece 1
├── Block 0 → not requested
├── Block 1 → not requested
└── ...
```

---

# Piece Selection

The client needs to decide:

> Which piece should I request next?

The implementation uses a basic **rarest-first** strategy.

Imagine three peers:

```text
Peer A → Pieces 0, 1, 2
Peer B → Pieces 1, 2, 3
Peer C → Pieces 2, 3, 4
```

Availability:

```text
Piece 0 → 1 peer
Piece 1 → 2 peers
Piece 2 → 3 peers
Piece 3 → 2 peers
Piece 4 → 1 peer
```

Rarest pieces:

```text
Piece 0
Piece 4
```

Downloading rare pieces first helps prevent the client from becoming dependent on pieces that are available from very few peers.

---

# Bitfield

After connecting, a peer can send a **bitfield**.

The bitfield tells us which pieces that peer owns.

Example:

```text
Pieces:

0 1 2 3 4 5 6 7

Bitfield:

1 0 1 1 0 0 1 0
```

This means:

```text
Piece 0 ✓
Piece 1 ✗
Piece 2 ✓
Piece 3 ✓
Piece 4 ✗
Piece 5 ✗
Piece 6 ✓
Piece 7 ✗
```

The client uses this information to decide which requests can be sent to that peer.

---

# HAVE Messages

Peers can also send:

```text
HAVE
```

when they receive a new piece.

Example:

```text
Peer initially:

0 1 0 1 0

Peer downloads piece 2.

Peer sends:

HAVE 2
```

Now other peers know:

```text
0 1 1 1 0
```

The Piece Manager updates its knowledge of peer piece availability.

---

# `storage/storage.go`

The Storage layer writes downloaded data to disk.

When a block arrives:

```text
PIECE message
     │
     ▼
Piece index
     │
     ▼
Block offset
     │
     ▼
Storage
     │
     ▼
File
```

For a single-file torrent:

```text
pieceIndex * pieceLength + begin
```

determines the position in the file.

Example:

```text
pieceIndex  = 5
pieceLength = 1 MB
begin       = 32 KB
```

The block starts at:

```text
5 × 1 MB + 32 KB
```

The Storage layer uses random-access writes so blocks can arrive **out of order**.

---

# Why Blocks Can Arrive Out of Order

Suppose we need:

```text
Piece 0:
Block A
Block B
Block C
Block D
```

But the network returns:

```text
Block C
Block A
Block D
Block B
```

That's completely fine.

The client knows the correct location from:

```text
piece index
+
block offset
```

So it writes:

```text
Block C → correct position
Block A → correct position
Block D → correct position
Block B → correct position
```

Eventually the entire piece is reconstructed.

---

# Piece Verification

Every piece has an expected SHA-1 hash stored in the torrent metadata.

The process is:

```text
Downloaded piece
       │
       ▼
     SHA-1
       │
       ▼
   Actual hash
```

Compare it with:

```text
Expected hash
```

Then:

```text
Actual == Expected
        │
     ┌──┴──┐
     │     │
    YES    NO
     │     │
     ▼     ▼
   Valid  Invalid
```

If valid:

```text
Piece complete ✓
```

If invalid:

```text
Piece rejected
       │
       ▼
Download again
```

This prevents corrupted data from being treated as a valid piece.

---

# `download/manager.go`

This package coordinates the entire download process.

It connects:

```text
Tracker
   │
   ▼
Peers
   │
   ▼
Piece Manager
   │
   ▼
Storage
```

Responsibilities include:

* Managing peers
* Creating peer connections
* Starting download workers
* Handling blocks
* Writing blocks
* Verifying pieces
* Reconnecting to peers
* Determining when the download is complete

Think of it as the **Download Controller**.

---

# How One Block Travels Through the System

This is one of the most important data flows.

```text
             PEER
              │
              │ PIECE message
              ▼
      ┌──────────────────┐
      │ peer/connection  │
      └────────┬─────────┘
               │
               │ index
               │ begin
               │ data
               ▼
      ┌──────────────────┐
      │ pieces/manager   │
      └────────┬─────────┘
               │
               │ StoreBlock()
               ▼
      ┌──────────────────┐
      │     storage      │
      └────────┬─────────┘
               │
               ▼
             Disk/File
```

After enough blocks arrive:

```text
Blocks
 │
 ├── Block 0 ✓
 ├── Block 1 ✓
 ├── Block 2 ✓
 └── Block 3 ✓
        │
        ▼
   Piece Complete
        │
        ▼
       SHA-1
        │
        ▼
      Verified
```

---

# Multiple Peers

The client does not download everything from one peer.

Instead:

```text
                   Torrent
                      │
                Peer Discovery
                      │
          ┌───────────┼───────────┐
          ▼           ▼           ▼
        Peer A      Peer B      Peer C
          │           │           │
          ▼           ▼           ▼
       Piece 0      Piece 3      Piece 7
       Piece 2      Piece 4      Piece 8
          │           │           │
          └───────────┼───────────┘
                      ▼
                 Piece Manager
                      │
                      ▼
                    Disk
```

This is what makes BitTorrent a **peer-to-peer protocol**.

---

# Why Multiple Peers Are Important

Imagine:

```text
File = 100 MB
```

Instead of:

```text
Server
  │
  │ 100 MB
  ▼
Client
```

BitTorrent can do:

```text
Peer A → 20 MB
Peer B → 30 MB
Peer C → 25 MB
Peer D → 25 MB
           │
           ▼
         Client
          100 MB
```

The client can download different pieces simultaneously.

---

# Complete Package Relationship

```text
                     cmd/client
                          │
                          ▼
                       download
                      /    |    \
                     /     |     \
                    ▼      ▼      ▼
                tracker  peer   pieces
                   │       │       │
                   │       │       │
                   └───────┼───────┘
                           │
                           ▼
                        storage
```

The torrent package provides metadata:

```text
torrent
  │
  ├── bencode.go
  ├── metainfo.go
  └── files.go
       │
       │ metadata
       ▼
    download
```

---

# End-to-End Example

Suppose we run:

```bash
./bittorrent -torrent ubuntu.torrent -output ./downloads
```

## Step 1 — CLI

`main.go` reads:

```text
ubuntu.torrent
./downloads
```

---

## Step 2 — Parse Torrent

`metainfo.go` loads the torrent.

It discovers information such as:

```text
Name:
ubuntu.iso

Piece Length:
4 MB

Pieces:
500 SHA-1 hashes

Tracker:
http://tracker.example.com/announce
```

---

## Step 3 — Calculate Info Hash

The client calculates:

```text
SHA1(Bencoded(info))
```

Result:

```text
info_hash
```

---

## Step 4 — Contact Tracker

The tracker receives:

```text
info_hash
peer_id
downloaded
uploaded
left
```

The tracker returns:

```text
Peer A
Peer B
Peer C
Peer D
```

---

## Step 5 — Connect to Peers

The client opens TCP connections:

```text
Client ──── Peer A
Client ──── Peer B
Client ──── Peer C
Client ──── Peer D
```

---

## Step 6 — Handshake

Each connection performs:

```text
Client → Handshake
Peer   → Handshake
```

The peer verifies the same:

```text
info_hash
```

---

## Step 7 — Receive Piece Availability

Peers send:

```text
BITFIELD
```

or:

```text
HAVE
```

The client learns:

```text
Peer A → pieces 0,1,4,7
Peer B → pieces 1,2,5,8
Peer C → pieces 2,3,5,9
```

---

## Step 8 — Send Interested

The client tells peers:

```text
INTERESTED
```

---

## Step 9 — Wait for Unchoke

Peer responds:

```text
UNCHOKE
```

Now the client can request data.

---

## Step 10 — Select a Piece

The Piece Manager chooses a piece.

For example:

```text
Piece 7
```

---

## Step 11 — Split Piece into Blocks

```text
Piece 7

├── Block 0
├── Block 1
├── Block 2
├── Block 3
└── ...
```

---

## Step 12 — Send Requests

The client sends:

```text
REQUEST
index=7
begin=0
length=16384
```

Then:

```text
REQUEST
index=7
begin=16384
length=16384
```

And so on.

---

## Step 13 — Receive Piece Messages

The peer sends:

```text
PIECE
index=7
begin=0
data=...
```

The client stores the block.

---

## Step 14 — Write to Disk

The Storage layer writes the block at the correct offset.

---

## Step 15 — Complete Piece

When every block is received:

```text
Piece 7
├── Block 0 ✓
├── Block 1 ✓
├── Block 2 ✓
└── Block 3 ✓
```

The piece is complete.

---

## Step 16 — Verify SHA-1

```text
Downloaded Piece
       │
       ▼
     SHA-1
       │
       ▼
Compare with torrent hash
```

If correct:

```text
✓ Piece verified
```

---

## Step 17 — Continue

The client repeats:

```text
select piece
     ↓
select block
     ↓
request
     ↓
receive
     ↓
write
     ↓
verify
```

until:

```text
All pieces verified
```

---

# The Core Loop

At a high level, the client repeatedly performs:

```text
┌─────────────────────────┐
│ Find available peers    │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Connect to peer         │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Perform handshake       │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Learn peer pieces       │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Wait for unchoke        │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Select piece            │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Select block            │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Request block           │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Receive block           │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Write block             │
└────────────┬────────────┘
             ▼
┌─────────────────────────┐
│ Piece complete?         │
└───────┬─────────┬───────┘
        │ No      │ Yes
        │         ▼
        │    SHA-1 verify
        │         │
        │         ▼
        │    Piece valid?
        │      /      \
        │    No        Yes
        │    │          │
        │    ▼          ▼
        │  Retry      Complete
        │               │
        └───────────────┘
```

---

# Current Architecture

| Package      | Responsibility                          |
| ------------ | --------------------------------------- |
| `cmd/client` | CLI and application entry point         |
| `torrent`    | Torrent metadata and Bencode            |
| `tracker`    | Peer discovery                          |
| `peer`       | Peer connections and protocol messages  |
| `pieces`     | Piece/block scheduling and verification |
| `storage`    | Writing data to disk                    |
| `download`   | Coordinates the download                |

This separation makes it easier to improve individual components without rewriting the entire client.

---

# Current Feature Set

### Implemented

* Bencode decoding
* Bencode encoding
* `.torrent` parsing
* Info hash generation
* HTTP tracker support
* UDP tracker support
* Compact peer parsing
* TCP peer connections
* BitTorrent handshake
* Bitfield handling
* HAVE handling
* CHOKE / UNCHOKE
* INTERESTED
* REQUEST
* PIECE
* Piece/block scheduling
* Multiple peers
* Basic rarest-first selection
* SHA-1 verification
* Single-file torrents
* Multi-file torrent metadata
* Basic peer reconnection

---

# Future Improvements

The project can be extended toward a more complete BitTorrent implementation.

```text
Current Client
      │
      ├── DHT
      │
      ├── PEX
      │
      ├── Magnet Links
      │
      ├── Incoming Connections
      │
      ├── Upload / Seeding
      │
      ├── Pause / Resume
      │
      ├── Persistent Download State
      │
      ├── Better Reconnection
      │
      ├── Endgame Mode
      │
      ├── Rarest-First Improvements
      │
      ├── NAT Traversal
      │
      └── WebRTC / Modern Transport
```

---

# Learning Goals

This project is not only about downloading files.

It demonstrates several important systems concepts.

## Networking

```text
TCP
UDP
Sockets
Binary protocols
Network timeouts
Connection management
```

## Distributed Systems

```text
Peer discovery
Multiple peers
Fault tolerance
Retries
Eventually consistent peer information
```

## Storage

```text
Random-access writes
File offsets
Piece/block mapping
Multi-file storage
```

## Data Structures

```text
Piece states
Block queues
Peer availability
Scheduling
```

## Cryptography

```text
SHA-1
Content verification
Info hash
```

## Concurrency

Go makes it possible to manage multiple peers concurrently:

```text
                 Download Manager
                        │
              ┌─────────┼─────────┐
              ▼         ▼         ▼
          Goroutine  Goroutine  Goroutine
            Peer A     Peer B     Peer C
              │         │         │
              └─────────┼─────────┘
                        ▼
                   Piece Manager
                        │
                        ▼
                       Disk
```

---

# Running the Client

## Build

```bash
go build -o bittorrent ./cmd/client
```

## Run

```bash
./bittorrent \
    -torrent ubuntu.torrent \
    -output ./downloads
```

## Windows

Build:

```powershell
go build -o bittorrent.exe ./cmd/client
```

Run:

```powershell
.\bittorrent.exe `
    -torrent big-buck-bunny.torrent `
    -output .\downloads
```

---

# Important Note

This is an **educational BitTorrent client**, not a production-ready replacement for mature clients such as qBittorrent or Transmission.

The goal is to understand how BitTorrent works internally:

```text
Torrent
   ↓
Bencode
   ↓
Info Hash
   ↓
Tracker
   ↓
Peers
   ↓
Handshake
   ↓
Bitfield / HAVE
   ↓
Pieces
   ↓
Blocks
   ↓
Requests
   ↓
Downloaded Data
   ↓
SHA-1 Verification
   ↓
Files
```

Building the client from these components makes the BitTorrent protocol much easier to understand than treating it as a black box.

---

Reference Blog: https://allenkim67.github.io/programming/2016/05/04/how-to-make-your-own-bittorrent-client.html
