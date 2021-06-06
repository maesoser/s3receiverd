package aws

import (
  "net/url"
  "crypto/sha256"
  "crypto/hmac"
  "strings"
  "net/http"
  "regexp"
  "sort"
  "fmt"
  "encoding/hex"
  "errors"
)

var (
	signV4Algorithm        = "AWS4-HMAC-SHA256"
	errNoAuthHeader        = errors.New("no authorization header present in request")
	errNoHashHeader        = errors.New("no hash header present in request")
	errAccKeyMismatch      = errors.New("invalid access key")
	errSignatureMismatch   = errors.New("invalid signature")
	errUnsignedHeaders     = errors.New("error processing signed headers")
	errMalformedAWSAuthorization = errors.New("malformed authorization header")
	authRegx               = regexp.MustCompile(`(?m)Credential=(.*)\/(\d{8})\/(.*)\/(.*)\/(.*), SignedHeaders=(.*), Signature=(.*)`)
)

type AWSClient struct {
	AccessKey   string
	SecretKey   string
}

type AWSAuthorization struct {
	AccessKey     string
	Date          string
	FullDate      string
	Region        string
	Service       string
	SignedHeaders []string
	Signature     string
	hashedPayload string
}

func ReadAWSAuthorization(req *http.Request) (AWSAuthorization, error) {
	header := AWSAuthorization{}
	authorization := req.Header.Get("authorization")
	if authorization == "" {
		return header, errNoAuthHeader
	}
	header.hashedPayload = req.Header.Get("x-amz-content-sha256")
	if header.hashedPayload == "" {
		return header,errNoHashHeader
	}
	matches := authRegx.FindStringSubmatch(authorization)
	if len(matches) != 8 {
		return header, errSignatureMismatch
	}
	header.AccessKey = matches[1]
	header.Date = matches[2]
	header.FullDate = req.Header.Get("X-Amz-Date")
	header.Region = matches[3]
	header.Service = matches[4]
	header.SignedHeaders = strings.Split(matches[6], ";")
	header.Signature = matches[7]
	return header, nil
}

func ValidateSignature(req *http.Request, secretKey, accessKey string) error {
    client := &AWSClient {
		AccessKey  : accessKey,
		SecretKey  : secretKey,
	}
	return client.ValidateV4(req)
}

func sum256(text []byte) string {
  b := sha256.Sum256(text)
  return hex.EncodeToString(b[:])
}

func contains(s []string, e string) bool {
    for _, a := range s {
        if a == e {
            return true
        }
    }
    return false
}

func getheader(req *http.Request, key string) string {
	if key == "host"{
		return req.Host
	}
    for k, vals := range(req.Header){
		for _, val := range(vals) {
			k := strings.ToLower(k)
			if k == key{
				return val
			}
		}
    }
    return ""
}

func hmacv4(key []byte, text string) []byte {
  mac := hmac.New(sha256.New, []byte(key))
  mac.Write([]byte(text))
  texthash := mac.Sum(nil)
  return texthash
}

func deriveKey(secret string, date string, region string, service string) []byte {
  return hmacv4(hmacv4(hmacv4(hmacv4([]byte("AWS4" + secret), date),region),service),"aws4_request")
}

func (c *AWSClient) ValidateV4(req *http.Request) error {
  auth, err := ReadAWSAuthorization(req);
  if err != nil {
	return err
  }
  if c.AccessKey != auth.AccessKey{
	  return errAccKeyMismatch
  }
  signedv := make([]string, 0)
  queryv := make([]string,0)      // query values
  headersv := make([]string, 0)   // header values
  queryValues := req.URL.Query()
  // build array of url query parameters
  for key, vals := range(queryValues) {
    for _, val := range(vals) {
      key = url.QueryEscape(key)
      val = url.QueryEscape(val)
      queryv = append(queryv, key+"="+val)
    }
  }
  for _, key := range(auth.SignedHeaders) {
	val := getheader(req, key)
	if val != ""{
		signedv = append(signedv, key)
      	headersv = append(headersv, key+":"+val)		
	}
  }

  // sort the arrays
  sort.Strings(signedv)
  signed := strings.Join(signedv, ";")
  sort.Strings(headersv)
  headers := strings.Join(headersv, "\n") + "\n"
  sort.Strings(queryv)
  query := strings.Join(queryv, "&")
  // construct the canonical request
  canreqv := []string{ req.Method, req.URL.Path, query, headers, signed, auth.hashedPayload }
  canreq := strings.Join(canreqv, "\n")
  canreqhash := sum256([]byte(canreq))
  // create the StringToSign
  stsv := []string {
    signV4Algorithm,
    auth.FullDate,
    fmt.Sprintf("%s/%s/%s/aws4_request", auth.Date, auth.Region, auth.Service),
    canreqhash,
  }
  sts := strings.Join(stsv, "\n")

  // derive the signing key...
  signingKey := deriveKey(c.SecretKey, auth.Date, auth.Region, auth.Service)

  // sign the the StringToSign 
  signature := hex.EncodeToString(hmacv4(signingKey, sts))

  if auth.Signature != signature{
	credential := fmt.Sprintf("%s/%s/%s/%s/aws4_request", c.AccessKey, auth.Date, auth.Region, auth.Service)
	authorization := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s, SignedHeaders=%s, Signature=%s", credential, signed, signature)
	fmt.Printf("Generated auth: %v\n", authorization)
	fmt.Printf("Received auth:  %v\n", req.Header.Get("Authorization"))
	  return errSignatureMismatch
  }

  return nil
}

