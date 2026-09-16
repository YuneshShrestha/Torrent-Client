/*
Storage represents the files you're going to download.
Example:

output/
└── Big Buck Bunny/
    ├── Big Buck Bunny.mp4
    ├── Big Buck Bunny.en.srt
    └── poster.jpg

Root: output/
Files: Information about each opened file

This handles both:

single-file torrent

and:

multi-file torrent

The important method is:

WriteAt(globalOffset)

because the torrent represents all files as one continuous byte stream.
*/

package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"torrent-client/torrent"
)

type Storage struct {
	Root string

	Files []OpenFile
}

/*
Each OpenFile tells the program:

File   -> actual opened file
Offset -> where this file starts in torrent's data
Length -> file size

Why Offset?

Because a torrent treats all files as one continuous byte stream.

Torrent data
─────────────────────────────────────
File A        File B          File C
──────────    ───────────     ───────
0             140             276...

The storage system needs to know which file corresponds to which part of that stream.
*/
type OpenFile struct {
	File   *os.File
	Offset int64
	Length int64
}

func New(
	root string,
	info *torrent.Info,
) (*Storage, error) {

	s := &Storage{
		Root: root,
	}

	/// This converts the torrent's file information into segments describing where each file belongs in the torrent's continuous data.
	segments := info.FileSegments()

	for _, segment := range segments {
		/*
			For example:

			root = "./downloads"
			segment.Path = ["Big Buck Bunny.mp4"]

			Result:
			./downloads/Big Buck Bunny.mp4

			For nested paths:
			segment.Path = ["folder", "video.mp4"]

			Result:
			./downloads/folder/video.mp4
		*/
		fullPath := filepath.Join(
			append(
				[]string{root},
				segment.Path...,
			)...,
		)

		if err := os.MkdirAll(
			filepath.Dir(fullPath),
			0755,
		); err != nil {
			return nil, err
		}

		/*
			O_CREATE -> create if it doesn't exist
			O_RDWR   -> allow reading + writing
		*/
		file, err := os.OpenFile(
			fullPath,
			os.O_CREATE|os.O_RDWR,
			0644,
		)

		if err != nil {
			return nil, err
		}

		/*
			`file.Truncate(segment.Length)`

			Suppose:

			Big Buck Bunny.mp4
			Length = 276,134,947 bytes

			The file is immediately created with that size.

			Conceptually:

			Big Buck Bunny.mp4

			[ empty ][ empty ][ empty ][ empty ]...
			<----------- 276 MB ------------>
		*/
		if err := file.Truncate(segment.Length); err != nil {
			file.Close()
			return nil, err
		}

		s.Files = append(
			s.Files,
			OpenFile{
				File:   file,
				Offset: segment.Offset,
				Length: segment.Length,
			},
		)
	}

	return s, nil
}

func (s *Storage) WriteAt(
	data []byte,
	offset int64,
) error {
	/*
		offset = 80 KB
		data   = 16 KB

		So end = 96 KB

		So write covers from 80 KB to 96 KB
	*/
	end := offset + int64(len(data))

	/*
		Imagine:
		file1:
		0 ───────── 50 KB

		file2:
		50 KB ───────── 150 KB

		file3:
		150 KB ───────── 350 KB

		Our data:

		80 KB ───────── 96 KB

		doesn't belong to file1.

		It belongs entirely to file2.

	*/
	for _, file := range s.Files {

		fileStart := file.Offset
		fileEnd := file.Offset + file.Length

		/// If the data doesnot overlap this file, skip the file
		if end <= fileStart ||
			offset >= fileEnd {
			continue
		}

		/// But if the data overlaps this file, then we need to write it
		writeStart := offset

		if writeStart < fileStart {
			writeStart = fileStart
		}

		writeEnd := end

		if writeEnd > fileEnd {
			writeEnd = fileEnd
		}

		sourceStart := writeStart - offset
		sourceEnd := writeEnd - offset

		n, err := file.File.WriteAt(
			/// Which bytes to write
			data[sourceStart:sourceEnd],
			/// Where to write
			writeStart-fileStart,
		)

		if err != nil {
			return err
		}

		expected := int(sourceEnd - sourceStart)

		if n != expected {
			return fmt.Errorf(
				"short write: %d/%d",
				n,
				expected,
			)
		}
	}

	return nil
}

func (s *Storage) Close() error {

	for _, file := range s.Files {
		if err := file.File.Close(); err != nil {
			return err
		}
	}

	return nil
}
