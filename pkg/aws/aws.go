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
)

type S3Server struct {
	Verbose   bool
	Aggregate bool
	Domain    string
	Secret    string
	AccessKey string
}

func (s *S3Server) ProcessRequest(w http.ResponseWriter, r *http.Request) {

	if s.Verbose == true {
		log.Printf("[ Verbose ] Received %s %s\n", r.Method, r.URL)
		log.Printf("[ Verbose ] Query %v\n", r.URL.Query())
		for name, headers := range r.Header {
			name = strings.ToLower(name)
			for _, h := range headers {
				log.Printf("[ Header ] %v: %v\n", name, h)
			}
		}
	}

	signer := Signer{}
	signer.Verbose = s.Verbose
	if err := signer.ReadAuthHeader(r); err != nil {
		log.Printf("[ Auth ] %s\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := signer.CheckAuthHeader(r, s.Secret, s.AccessKey); err != nil {
		log.Printf("[ Auth ] %s\n", err)
	}

	if r.Method == "GET" && strings.Contains(r.URL.Path, "/status") {
		fmt.Fprintf(w, "OK\n")
		w.WriteHeader(200)
	} else if r.Method == "PUT" && !strings.Contains(r.URL.RawQuery, "uploadId=") {
		s.processUpload(w, r)
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

func (s *S3Server) processUpload(w http.ResponseWriter, r *http.Request) {
	contentMD5 := r.Header.Get("content-md5")
	contentLengthStr := r.Header.Get("content-length")
	contentLength, err := strconv.Atoi(contentLengthStr)
	if err != nil {
		log.Panic(err)
	}
	bucket, filename := ParseS3Url(r.URL)
	log.Printf("Received %s/%s (%d bytes)\n", bucket, filename, contentLength)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}

	hash := md5.New() 
	io.WriteString(hash, string(body))
	newMD5 := b64.StdEncoding.EncodeToString(hash.Sum(nil))
    if contentMD5 != newMD5 {
    	log.Printf("[ MD5 ] Warning, MD5Sum mismatch\n")
    }
    
	if err := os.MkdirAll(bucket, os.ModePerm); err != nil {
		log.Printf("Error: %s\n", err)
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
			log.Println(err)
		}
		defer file.Close()
		if _, err := file.Write(body); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := ioutil.WriteFile(bucket+"/"+filename, body, 0644); err != nil {
			log.Printf("Error saving logs: %s\n", err)
			return
		}
	}

	w.Header().Set("Etag", contentMD5)
	w.WriteHeader(200)
}
