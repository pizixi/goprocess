package utils

import (
	"bufio"
	"io"
	"log"
	"os"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func CalculateOffset(file *os.File, byteCount int64, seekCount int64) int64 {
	if byteCount <= seekCount {
		return -byteCount
	}

	_, err := file.Seek(-seekCount, io.SeekEnd)
	if err != nil {
		log.Printf("文件定位失败: %v\n", err)
		return -seekCount
	}

	reader := bufio.NewReader(file)
	var offset int64 = -seekCount
	for {
		_, err := reader.ReadByte()
		if err != nil {
			break
		}
		offset++
		if reader.Buffered() > 0 {
			nextByte, _ := reader.Peek(1)
			if nextByte[0] == '\n' {
				break
			}
		}
	}

	return offset
}

func EnsureUTF8(data string) string {
	if utf8.ValidString(data) {
		return data
	}
	utf8Data, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), data)
	if err == nil {
		return utf8Data
	}
	byteData := []byte(data)
	encodings := []encoding.Encoding{
		unicode.UTF8,
		unicode.UTF16(unicode.BigEndian, unicode.UseBOM),
		unicode.UTF16(unicode.LittleEndian, unicode.UseBOM),
		simplifiedchinese.GBK,
		simplifiedchinese.GB18030,
		traditionalchinese.Big5,
		japanese.ShiftJIS,
		korean.EUCKR,
		charmap.ISO8859_1,
		charmap.ISO8859_2,
		charmap.ISO8859_3,
		charmap.ISO8859_4,
		charmap.ISO8859_5,
		charmap.ISO8859_6,
		charmap.ISO8859_7,
		charmap.ISO8859_8,
		charmap.ISO8859_9,
		charmap.ISO8859_10,
		charmap.ISO8859_13,
		charmap.ISO8859_14,
		charmap.ISO8859_15,
		charmap.ISO8859_16,
		charmap.Windows1250,
		charmap.Windows1251,
		charmap.Windows1252,
		charmap.Windows1253,
		charmap.Windows1254,
		charmap.Windows1255,
		charmap.Windows1256,
		charmap.Windows1257,
		charmap.Windows1258,
		charmap.KOI8R,
		charmap.KOI8U,
	}

	for _, enc := range encodings {
		ret, err := transformString(byteData, enc, unicode.UTF8)
		if err == nil {
			return ret
		}
	}

	return ""
}

func transformString(data []byte, src, dest encoding.Encoding) (string, error) {
	transformer := transform.Chain(src.NewDecoder(), dest.NewEncoder())
	res, _, err := transform.Bytes(transformer, data)
	if err != nil {
		return "", err
	}
	return string(res), nil
}
