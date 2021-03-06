package main

import (
  "bufio"
  "encoding/json"
  "fmt"
  "log"
  "io"
  "os"
  "strings"
  "strconv"
)

type GzipMetaData struct {
  FooterLength int `json:"Footer-Length,string"`
  DeflateLength int `json:"Deflate-Length,string"`
  HeaderLength int `json:"Header-Length,string"`
  InflatedLength int `json:"Inflated-Length,string"`
}

type WarcContainer struct {
  Compressed bool
  Offset int `json:",string"`
  Filename string
  GzipMeta GzipMetaData `json:"Gzip-Metadata"`
}

type WarcEnvelope struct {
  Format string
  HeaderLength int `json:"WARC-Header-Length,string"`
  Digest string `json:"Block-Digest"`
  ContentLength int `json:"Actual-Content-Length,string"`
  HeaderMetaData WarcMetaData `json:"WARC-Header-Metadata"`
  PayloadMetaData PayloadMetaData `json:"Payload-Metadata"`
}

type WarcMetaData struct {
  Type string `json:"WARC-Type"`
  Length int `json:"Content-Length,string"`
  RecordId string `json:"WARC-Record-ID"`
  Uri string `json:"WARC-Target-URI"`
  ContentType string `json:"Content-Type"`
}

type PayloadMetaData struct {
  ResponseMeta ResponseMetaData `json:"HTTP-Response-Metadata"`
}

type HTTPHeaders struct {
  ContentType string `json:"Content-Type"`
}

type ResponseMetaData struct {
  ResponseInfo ResponseMessage `json:"Response-Message"`
  Headers HTTPHeaders
}

type ResponseMessage struct {
  Status int `json:",string"`
}

type WarcMeta struct {
  Envelope WarcEnvelope
  Container WarcContainer
}

type record struct {
  length int
  refersTo string
  header []string
  warcType string
  data []byte
}

// Returns nil on EOF
func nextRecord(reader *bufio.Reader) (*record, error) {
  rec := record{}
  var failSafe int = 100
  var line string
  var err error

  /* We should always enter this function at the beginning of a record (possibly
     skipping blank lines */
  for {
    line, err = reader.ReadString('\n')
    if err != nil { return nil, err }
    line = strings.TrimSpace(line)
    if line == "WARC/1.0" {
      break
    } else if line == "" {
      continue
    } else {
      panic(fmt.Sprintf("Malformed first line: '%s'\n", line))
    }
  }
  rec.header = append(rec.header, line)

  // Get the rest of the header
  for {
    line, err = reader.ReadString('\n')
    if err != nil {
      if err == io.EOF { break }
      return nil, err
    }
    line = strings.TrimSpace(line)
    if line == "" { break }
    if strings.HasPrefix(line, "WARC-Type: ") {
      rec.warcType = line[11:]
    } else if strings.HasPrefix(line, "Content-Length: ") {
      rec.length, err = strconv.Atoi(line[16:])
      if err != nil { return nil, err }
    } else if strings.HasPrefix(line, "WARC-Refers-To: ") {
      rec.refersTo = line[16:]
    }
    rec.header = append(rec.header, line)
    failSafe--
    if failSafe == 0 { panic("Hit failsafe when reading header") }
  }

  // No read in the data
  if rec.length == 0 { panic("No record length") }
  rec.data = make([]byte, rec.length)
  var bytes int
  for bytes < rec.length {
    n, err := reader.Read(rec.data[bytes:])
    if err != nil { return nil, err }
    bytes += n
  }
  if bytes != rec.length {
    panic(fmt.Sprintf("Only read %d bytes, expecting %d\n", bytes, rec.length))
  }

  return &rec, err
}

/* Search reader for WARC meta records that refer to the records given by ids */
func readMeta(reader *bufio.Reader, ids *recordSet) {
  var rec *record
  var err error
  var meta WarcMeta
  var filename string
  for {
    rec, err = nextRecord(reader)
    if err != nil {
      if err == io.EOF {
        break
      }
      log.Fatal(err)
    }
    // Skip non-metadata records
    if rec.warcType != "metadata" { continue }
    // Skip records that are not the ones for which we're looking
    if rec.refersTo == "" { panic("No refersTo") }
    if !((*ids)[rec.refersTo]) { continue }
    if err := json.Unmarshal(rec.data, &meta); err != nil {
      log.Fatal(err)
    }
    if meta.Envelope.HeaderMetaData.Type == "response" {
      status := meta.Envelope.PayloadMetaData.ResponseMeta.ResponseInfo.Status
      if status != 200 { continue }
      contentType := meta.Envelope.PayloadMetaData.ResponseMeta.Headers.ContentType
      if strings.Contains(contentType, "text/html") {
        /* Each time we encounter a new filename we output it. Blank lines between
           records */
        if filename != meta.Container.Filename {
          if filename != "" {
            fmt.Println("")
          }
          filename = meta.Container.Filename
          fmt.Println(filename)
        }
        fmt.Println(meta.Container.Offset, meta.Container.GzipMeta.DeflateLength)
      }
    }
  }
}

type recordSet map[string]bool

/* Build a map of records that we'll look for in the metadata. These come
   from stdin. */
func getRecordSubset(fn string) *recordSet {
  set := make(recordSet, 100)
  file, err := os.Open(fn)
  if err != nil { log.Fatal(err) }
  defer func() {
    if err = file.Close(); err != nil {
      log.Fatal("Failed to close file")
    }
  }()
  reader := bufio.NewReader(file)
  for {
    line, err := reader.ReadString('\n')
    if err != nil {
      if err == io.EOF { break }
      log.Fatal(err)
    }
    id := strings.TrimSpace(line)
    set[id] = true
  }
  return &set
}

func main() {
  if len(os.Args) != 2 {
    log.Fatal("Must provide argument")
  }
  ids := getRecordSubset(os.Args[1])
  reader := bufio.NewReader(os.Stdin)
  readMeta(reader, ids)
}

// END
