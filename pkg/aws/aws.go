package aws

import (
    b64 "encoding/base64"
    "io"
    "crypto/md5"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"errors"
)

type S3Server struct {
	Verbose   bool
	Aggregate bool
	Domain    string
	Secret    string
	AccessKey string
	RootFolder    string
}

func (s *S3Server) ProcessRequest(w http.ResponseWriter, r *http.Request) {

	if s.Verbose == true {
		log.Printf("Received %s %s\n", r.Method, r.URL)
		log.Printf("Query %v\n", r.URL.Query())
		for name, headers := range r.Header {
			name = strings.ToLower(name)
			for _, h := range headers {
				log.Printf("Header: %v: %v\n", name, h)
			}
		}
	}

	if r.Method == "GET" {
		fmt.Fprintf(w, "ok\n")
        return
	} 

	if err := ValidateSignature(r, s.Secret, s.AccessKey); err != nil {
		log.Printf("Error: %s\n", err)
		return
	}

    if r.Method == "PUT" && !strings.Contains(r.URL.RawQuery, "uploadId=") {
		err := s.processUpload(w, r)
		if err != nil{
			log.Println(err)
		}
	} else if r.Method == "PUT" && strings.Contains(r.URL.RawQuery, "uploadId=") {
		s.processUploadPart(w, r)
	} else if r.Method == "POST" && r.URL.RawQuery == "uploads" {
		s.processMultipartInitiate(w, r)
	} else if r.Method == "POST" && strings.Contains(r.URL.RawQuery, "uploadId=") {
		s.processMultipartComplete(w, r)
	} else {
		log.Printf("Unknown request %s %s\n", r.Method, r.URL.String())
		w.WriteHeader(200)
	}

}

func (s *S3Server) processUpload(w http.ResponseWriter, r *http.Request) error {
	
	contentLengthStr := r.Header.Get("content-length")
	contentLength, err := strconv.Atoi(contentLengthStr)
	if err != nil {
		return err
	}
	bucket, filename := ParseS3Url(r.URL)
	if s.RootFolder != ""{
		bucket = s.RootFolder
	}
	log.Printf("Received %s/%s (%d bytes)\n", bucket, filename, contentLength)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "can't read body", http.StatusBadRequest)
		return err
	}

	hash := md5.New() 
	io.WriteString(hash, string(body))
	newMD5 := b64.StdEncoding.EncodeToString(hash.Sum(nil))
	contentMD5 := r.Header.Get("content-md5")
    if contentMD5 != newMD5 {
		return errors.New("invalid md5")
    }

	// /s3/20210503/20210503T210124Z_20210503T210154Z_0d4c0d2b.log.gz
	// Received /20210503/20210503T210624Z_20210503T210654Z_126b8a6b.log.gz
    
	if err := os.MkdirAll(bucket, os.ModePerm); err != nil {
		return err
	}

	if filename[len(filename)-3:] == ".gz" {
		data, err := Uncompress(body)
		if err != nil {
			log.Printf("Error uncrompressing logs: %s\n", err)
		} else {
			filename = filename[:len(filename)-3]
			body = data
		}

	}

	if s.Aggregate {
		filename = time.Now().Format("20060201") + ".log"
		file, err := os.OpenFile(bucket+"/"+filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer file.Close()
		if _, err := file.Write(body); err != nil {
			return err
		}
	} else {
		if err := ioutil.WriteFile(bucket+"/"+filename, body, 0644); err != nil {
			return err
		}
	}

	w.Header().Set("Etag", contentMD5)
	w.WriteHeader(200)
	return nil
}
