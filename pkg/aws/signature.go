package aws

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	signV4Algorithm        = "AWS4-HMAC-SHA256"
	errInvalidMethod       = errors.New("signer only handles HTTP POST")
	errNoAuthHeader        = errors.New("no authorization header present in request")
	errNoHashHeader        = errors.New("no hash header present in request")
	errAccKeyMismatch      = errors.New("invalid access key")
	errSignatureMismatch   = errors.New("invalid signature")
	errUnsignedHeaders     = errors.New("error processing signed headers")
	errMalformedAuthHeader = errors.New("malformed authorization header")
	authRegx               = regexp.MustCompile(`(?m)Credential=(.*)\/(\d{8})\/(.*)\/(.*)\/(.*), SignedHeaders=(.*), Signature=(.*)`)
	reservedObjectNames    = regexp.MustCompile("^[a-zA-Z0-9-_.~/]+$")
)

const (
	signatureVersion = "2"
	signatureMethod  = "HmacSHA256"
	timeFormat       = "2006-01-02T15:04:05Z"
)

type Signer struct {
	Request       *http.Request
	Time          time.Time
	AccessKey     string
	Date          string
	Region        string
	Service       string
	SignedHeaders []string
	Signature     string
	hashedPayload string

	Verbose bool
}

func (v2 *Signer) ReadAuthHeader(req *http.Request) error {
	authorization := req.Header.Get("authorization")
	if authorization == "" {
		return errNoAuthHeader
	}
	v2.hashedPayload = req.Header.Get("x-amz-content-sha256")
	if v2.hashedPayload == "" {
		return errNoHashHeader
	}

	matches := authRegx.FindStringSubmatch(authorization)
	if len(matches) != 8 {
		return errMalformedAuthHeader
	}
	v2.AccessKey = matches[1]
	v2.Date = matches[2]
	v2.Region = matches[3]
	v2.Service = matches[4]
	v2.SignedHeaders = strings.Split(matches[6], ";")
	v2.Signature = matches[7]
	return nil
}

// EncodePath encode the strings from UTF-8 byte representations to HTML hex escape sequences
//
// This is necessary since regular url.Parse() and url.Encode() functions do not support UTF-8
// non english characters cannot be parsed due to the nature in which url.Encode() is written
//
// This function on the other hand is a direct replacement for url.Encode() technique to support
// pretty much every UTF-8 character.
func EncodePath(pathName string) string {
	if reservedObjectNames.MatchString(pathName) {
		return pathName
	}
	var encodedPathname string
	for _, s := range pathName {
		if 'A' <= s && s <= 'Z' || 'a' <= s && s <= 'z' || '0' <= s && s <= '9' { // §2.3 Unreserved characters (mark)
			encodedPathname = encodedPathname + string(s)
			continue
		}
		switch s {
		case '-', '_', '.', '~', '/': // §2.3 Unreserved characters (mark)
			encodedPathname = encodedPathname + string(s)
			continue
		default:
			len := utf8.RuneLen(s)
			if len < 0 {
				// if utf8 cannot convert return the same string as is
				return pathName
			}
			u := make([]byte, len)
			utf8.EncodeRune(u, s)
			for _, r := range u {
				hex := hex.EncodeToString([]byte{r})
				encodedPathname = encodedPathname + "%" + strings.ToUpper(hex)
			}
		}
	}
	return encodedPathname
}

// Trim leading and trailing spaces and replace sequential spaces with one space, following Trimall()
// in http://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
func signV4TrimAll(input string) string {
	// Compress adjacent spaces (a space is determined by
	// unicode.IsSpace() internally here) to one space and return
	return strings.Join(strings.Fields(input), " ")
}

// getCanonicalHeaders generate a list of request headers with their values
func getCanonicalHeaders(signedHeaders http.Header) string {
	var headers []string
	vals := make(http.Header)
	for k, vv := range signedHeaders {
		headers = append(headers, strings.ToLower(k))
		vals[strings.ToLower(k)] = vv
	}
	sort.Strings(headers)

	var buf bytes.Buffer
	for _, k := range headers {
		buf.WriteString(k)
		buf.WriteByte(':')
		for idx, v := range vals[k] {
			if idx > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(signV4TrimAll(v))
		}
		buf.WriteByte('\n')
	}
	return buf.String()
}

// getSignedHeaders generate a string i.e alphabetically sorted, semicolon-separated list of lowercase request header names
func getSignedHeaders(signedHeaders http.Header) string {
	var headers []string
	for k := range signedHeaders {
		headers = append(headers, strings.ToLower(k))
	}
	sort.Strings(headers)
	return strings.Join(headers, ";")
}

// getCanonicalRequest generate a canonical request of style
//
// canonicalRequest =
//  <HTTPMethod>\n
//  <CanonicalURI>\n
//  <CanonicalQueryString>\n
//  <CanonicalHeaders>\n
//  <SignedHeaders>\n
//  <HashedPayload>
//
func getCanonicalRequest(extractedSignedHeaders http.Header, payload, queryStr, urlPath, method string) string {
	rawQuery := strings.Replace(queryStr, "+", "%20", -1)
	encodedPath := EncodePath(urlPath)
	canonicalRequest := strings.Join([]string{
		method,
		encodedPath,
		rawQuery,
		getCanonicalHeaders(extractedSignedHeaders),
		getSignedHeaders(extractedSignedHeaders),
		payload,
	}, "\n")
	return canonicalRequest
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// extractSignedHeaders extract signed headers from Authorization header
func extractSignedHeaders(signedHeaders []string, r *http.Request) (http.Header, error) {
	reqHeaders := r.Header
	reqQueries := r.URL.Query()
	// find whether "host" is part of list of signed headers.
	// if not return ErrUnsignedHeaders. "host" is mandatory.
	if !contains(signedHeaders, "host") {
		return nil, errUnsignedHeaders
	}
	extractedSignedHeaders := make(http.Header)
	for _, header := range signedHeaders {
		// `host` will not be found in the headers, can be found in r.Host.
		// but its alway necessary that the list of signed headers containing host in it.
		val, ok := reqHeaders[http.CanonicalHeaderKey(header)]
		if !ok {
			// try to set headers from Query String
			val, ok = reqQueries[header]
		}
		if ok {
			for _, enc := range val {
				extractedSignedHeaders.Add(header, enc)
			}
			continue
		}
		switch header {
		case "expect":
			extractedSignedHeaders.Set(header, "100-continue")
		case "host":
			extractedSignedHeaders.Set(header, r.Host)
		case "transfer-encoding":
			for _, enc := range r.TransferEncoding {
				extractedSignedHeaders.Add(header, enc)
			}
		case "content-length":
			extractedSignedHeaders.Set(header, strconv.FormatInt(r.ContentLength, 10))
		default:
			return nil, errUnsignedHeaders
		}
	}
	return extractedSignedHeaders, nil
}

// sumHMAC calculate hmac between two input byte array.
func sumHMAC(key []byte, data []byte) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write(data)
	return hash.Sum(nil)
}

// https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-header-based-auth.html
func (v2 *Signer) CheckAuthHeader(req *http.Request, secretKey, accessKey string) error {

	if v2.AccessKey != accessKey {
		return errAccKeyMismatch
	}

	// Extract all the signed headers along with its values.
	signedHeaders, errCode := extractSignedHeaders(v2.SignedHeaders, req)
	if errCode != nil {
		return errCode
	}
	// Query string.
	queryStr := req.URL.Query().Encode()

	// Get canonical request.
	canonicalRequest := getCanonicalRequest(signedHeaders, v2.hashedPayload, queryStr, req.URL.Path, req.Method)

	// Get string to sign from canonical request.
	stringToSign := signV4Algorithm + "\n" + v2.Date + "\n"
	scope := strings.Join([]string{v2.Date, v2.Region, v2.Service, "aws4_request"}, "/")
	stringToSign = stringToSign + scope + "\n"
	canonicalRequestBytes := sha256.Sum256([]byte(canonicalRequest))
	stringToSign = stringToSign + hex.EncodeToString(canonicalRequestBytes[:])

	// Get hmac signing key.
	date := sumHMAC([]byte("AWS4"+secretKey), []byte(v2.Date))
	regionBytes := sumHMAC(date, []byte(v2.Region))
	service := sumHMAC(regionBytes, []byte(v2.Service))
	signingKey := sumHMAC(service, []byte("aws4_request"))

	// Calculate signature.
	newSignature := hex.EncodeToString(sumHMAC(signingKey, []byte(stringToSign)))
	//log.Printf("STR: %s\n", stringToSign)
	//log.Printf("KEY: %s\n", signingKey)
	//log.Printf("SIG: %s\n", newSignature)
	//log.Printf("OLDSIG: %s\n", v2.Signature)

	if subtle.ConstantTimeCompare([]byte(v2.Signature), []byte(newSignature)) != 1 {
		return errSignatureMismatch
	}

	return nil
}
