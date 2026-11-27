package rw

import (
	"io"
	"math"

	"github.com/metacubex/sing/common"
	"github.com/metacubex/sing/common/binary"
	E "github.com/metacubex/sing/common/exceptions"
)

const MaxVStringLen = 1 << 20 // 1 MiB default upper bound for ReadVString

type stubByteReader struct {
	io.Reader
}

func (r stubByteReader) ReadByte() (byte, error) {
	var b [1]byte
	var n int
	var err error
	for n == 0 && err == nil {
		n, err = r.Read(b[:])
	}

	if n == 1 && err == io.EOF {
		err = nil
	}
	return b[0], err
}

func ToByteReader(reader io.Reader) io.ByteReader {
	if byteReader, ok := reader.(io.ByteReader); ok {
		return byteReader
	}
	return &stubByteReader{reader}
}

func ReadUVariant(reader io.Reader) (uint64, error) {
	return binary.ReadUvarint(ToByteReader(reader))
}

func UvarintLen(x uint64) int {
	switch {
	case x < 1<<(7*1):
		return 1
	case x < 1<<(7*2):
		return 2
	case x < 1<<(7*3):
		return 3
	case x < 1<<(7*4):
		return 4
	case x < 1<<(7*5):
		return 5
	case x < 1<<(7*6):
		return 6
	case x < 1<<(7*7):
		return 7
	case x < 1<<(7*8):
		return 8
	case x < 1<<(7*9):
		return 9
	default:
		return 10
	}
}

func WriteUVariant(writer io.Writer, value uint64) error {
	var b [binary.MaxVarintLen64]byte
	return common.Error(writer.Write(b[:binary.PutUvarint(b[:], value)]))
}

func WriteVString(writer io.Writer, value string) error {
	err := WriteUVariant(writer, uint64(len(value)))
	if err != nil {
		return err
	}
	return WriteString(writer, value)
}

func ReadVString(reader io.Reader) (string, error) {
	return ReadVStringLimit(reader, MaxVStringLen)
}

func ReadVStringLimit(reader io.Reader, maxLen int) (string, error) {
	length, err := ReadUVariant(reader)
	if err != nil {
		return "", err
	}
	if maxLen < 0 {
		maxLen = 0
	}
	if length > uint64(maxLen) || length > math.MaxInt {
		return "", E.New("vstring length exceeds limit: ", length)
	}
	if length == 0 {
		return "", nil
	}
	return ReadString(reader, int(length))
}
