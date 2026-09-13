/*This defines a block request.*/

package pieces

const BlockSize int64 = 16 * 1024

type Block struct {
	Piece  int
	Begin  int64
	Length int64
}

func BlocksForPiece(
	piece int,
	pieceLength int64,
	totalLength int64,
) []Block {

	start := int64(piece) * pieceLength

	remaining := totalLength - start

	if remaining <= 0 {
		return nil
	}

	actualPieceLength := pieceLength

	if remaining < actualPieceLength {
		actualPieceLength = remaining
	}

	var result []Block

	for begin := int64(0); begin < actualPieceLength; begin += BlockSize {

		length := BlockSize

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
