/*
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

	segments := info.FileSegments()

	for _, segment := range segments {

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

		file, err := os.OpenFile(
			fullPath,
			os.O_CREATE|os.O_RDWR,
			0644,
		)

		if err != nil {
			return nil, err
		}

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

	end := offset + int64(len(data))

	for _, file := range s.Files {

		fileStart := file.Offset
		fileEnd := file.Offset + file.Length

		if end <= fileStart ||
			offset >= fileEnd {
			continue
		}

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
			data[sourceStart:sourceEnd],
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
