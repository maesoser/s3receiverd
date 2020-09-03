package aws

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"hash/crc32"
	"io"
	"math/rand"
	"net/url"
	"strings"
	"time"
)

func ParseS3Url(url *url.URL) (string, string) {
	urlSlice := strings.Split(url.Path, "/")
	if len(urlSlice) < 2 {
		return "", ""
	}
	bucket := strings.Join(urlSlice[2:len(urlSlice)-1], "/")
	filename := urlSlice[len(urlSlice)-1]
	return "/" + bucket, filename
}

func genUploadID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 58)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func genEtag(data []byte) string {
	crc := crc32.ChecksumIEEE(data)
	return fmt.Sprintf(`W/"%d%08X"`, len(data), crc)
}

func Uncompress(data []byte) (resData []byte, err error) {
	b := bytes.NewBuffer(data)

	var r io.Reader
	r, err = gzip.NewReader(b)
	if err != nil {
		return
	}

	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		return
	}

	resData = resB.Bytes()

	return
}
