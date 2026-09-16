/*This defines a block request.*/

package pieces

/// BlockSize = 16384 bytes = 16 KiB
const BlockSize int64 = 16 * 1024

type Block struct {
	Piece  int   /// Which piece [Ex: Piece 2]
	Begin  int64 /// Where inside the piece does this block start
	Length int64 /// How many bytes should we request
}

func BlocksForPiece(
	piece int, /// Which piece
	pieceLength int64, /// Normal piece size
	totalLength int64, /// Entire torrent Size
) []Block {
	/// Tells where this piece begins in the whole torrent
	start := int64(piece) * pieceLength

	/// Calcualte how much data remains
	remaining := totalLength - start

	/*
		If:
		start >= totalLength
		there's no data available for this piece.
	*/
	if remaining <= 0 {
		return nil
	}

	actualPieceLength := pieceLength

	// This is specific to the last piece
	// If the last piece is smaller than the normal piece size
	if remaining < actualPieceLength {
		actualPieceLength = remaining
	}

	var result []Block

	/// Loop through the piece
	for begin := int64(0); begin < actualPieceLength; begin += BlockSize {

		length := BlockSize

		/*
			Handle the final block

			This prevents requesting data beyond the end of the piece

			Suppose actual piece length = 20000

			The blocks should be:
			Block 1:
			begin  = 0
			length = 16384

			Block 2:
			begin  = 16384
			length = 3616

		*/
		if begin+length > actualPieceLength {
			length = actualPieceLength - begin
		}

		result = append(result, Block{
			Piece:  piece,
			Begin:  begin,
			Length: length,
		})
	}

	return result
}
