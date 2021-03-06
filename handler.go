package pdf

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

// ConvertRequest encapsulates wkhtmltopdf's converter and object options
type ConvertRequest struct {
	ConverterOpts *ConverterOpts `json:"converterOpts"`
	ObjectOpts    *ObjectOpts    `json:"objectOpts"`
}

// ConvertRequestChannel receives request for convertion
var ConvertRequestChannel = make(chan ConvertRequest)

// ConvertResponseChannel delivers converted content
var ConvertResponseChannel = make(chan []byte)

// StopConvertLoopChannel informs stop signal
var StopConvertLoopChannel = make(chan bool)
var stopConvertLoop = false

// StartConvertLoop is the main thread loop listen to ConvertRequestChannel
// and feeding ConvertResponseChannel
func StartConvertLoop() {
	log.Println("Starting convert loop")

	Init()
	defer Destroy()

	go func() {
		<-StopConvertLoopChannel
		log.Println("Received StopConvertLoop signal")
		stopConvertLoop = true
	}()

	for !stopConvertLoop {
		log.Println("Waiting for convertion request...")
		request := <-ConvertRequestChannel
		log.Println("Received a convertion request for:", request.ObjectOpts.Location)

		content, err := convert(request)
		if err != nil {
			log.Println("Failed to convert:", request.ObjectOpts.Location)
			ConvertResponseChannel <- nil
		}

		log.Println("Sending PDF content:", request.ObjectOpts.Location)
		ConvertResponseChannel <- content
	}

	log.Println("Convert loop is over")
}

// StopConvertLoop sends a StopConvertLoop signal
func StopConvertLoop() {
	log.Println("Sending StopConvertLoop signal")
	StopConvertLoopChannel <- true
}

// ConvertPostHandler converts HTML to PDF based on payload options
func ConvertPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		respondWithText(w, r, http.StatusMethodNotAllowed, "Invalid request method")
		return
	}

	log.Println("--- BEGIN REQUEST ---")

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	request := ConvertRequest{
		ConverterOpts: NewConverterOpts(),
		ObjectOpts:    NewObjectOpts(),
	}
	if err := decoder.Decode(&request); err != nil {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			body = make([]byte, 0, 0)
		}
		respondWithText(w, r, http.StatusBadRequest, string(body))
		return
	}

	log.Println("Requesting for convert:", request.ObjectOpts.Location)
	ConvertRequestChannel <- request
	content := <-ConvertResponseChannel
	log.Println("Received response for:", request.ObjectOpts.Location)

	if content == nil {
		respondWithText(w, r, http.StatusInternalServerError, "Failed to convert file")
		return
	}
	respondWithPDF(w, r, http.StatusOK, content)

	log.Println("--- END REQUEST ---")
}

func respondWithText(w http.ResponseWriter, r *http.Request, statusCode int, payload string) {
	w.WriteHeader(statusCode)
	w.Write([]byte(payload))
}

func respondWithPDF(w http.ResponseWriter, r *http.Request, statusCode int, payload []byte) {
	w.WriteHeader(statusCode)
	w.Header().Add("content-type", "application/pdf")
	w.Write(payload)
}

func convert(request ConvertRequest) ([]byte, error) {
	// Create object from url
	object, err := NewObjectWithOpts(request.ObjectOpts)
	if err != nil {
		log.Println("Could not create object for", request.ObjectOpts.Location)
		return nil, err
	}
	log.Println("Object URL:", request.ObjectOpts.Location)

	// Create converter
	converter, err := NewConverterWithOpts(request.ConverterOpts)
	if err != nil {
		log.Println("Could not create converter for", request.ObjectOpts.Location)
		return nil, err
	}
	defer converter.Destroy()

	// Add created object to the converter
	converter.Add(object)

	// Convert the objects and get the output PDF document
	output := new(bytes.Buffer)
	err = converter.Run(output)
	if err != nil {
		log.Println("Could not convert object to PDF:", request.ObjectOpts.Location)
		return nil, err
	}
	raw := output.Bytes()
	log.Println("PDF", len(raw), "bytes of size:", request.ObjectOpts.Location)

	return raw, nil
}
