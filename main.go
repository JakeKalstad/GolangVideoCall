package main

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nats-io/nats.go"
	"nhooyr.io/websocket"
)

var natsConn *nats.Conn

func Initialize() error {
	var err error
	natsConn, err = nats.Connect(nats.DefaultURL)
	if err != nil {
		return err
	}
	return nil
}

func wsLoop(ctx context.Context, cancelFunc context.CancelFunc, ws *websocket.Conn, topic string, userID string) {
	defer closeWS(ws)
	for {
		if _, message, err := ws.Read(ctx); err != nil {
			log.Printf("Error reading message %s", err)
			break
		} else {
			msg := &nats.Msg{Subject: topic, Data: message, Reply: userID}
			if err = natsConn.PublishMsg(msg); err != nil {
				log.Printf("Could not publish message: %s", err)
				return
			}
		}

	}
	cancelFunc()
}

func natsConnLoop(cctx, ctx context.Context, ws *websocket.Conn, topic string, userID string) {
	_, err := natsConn.Subscribe(topic, func(m *nats.Msg) {
		m.Ack()
		if m.Reply == userID {
			return
		}
		if err := ws.Write(ctx, websocket.MessageText, m.Data); err != nil {
			log.Printf("Error writing message to %s: %s", userID, err)
			return
		}
	})
	if err != nil {
		panic(err)
	}
}

func closeWS(ws *websocket.Conn) {
	// can check if already closed here
	if err := ws.Close(websocket.StatusNormalClosure, ""); err != nil {
		log.Printf("Error closing: %s", err)
	}
}

func VideoConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	userID := strings.ToLower(r.URL.Query().Get("userID"))
	peerID := strings.ToLower(r.URL.Query().Get("peerID"))
	peers := []string{userID, peerID}
	sort.Strings(peers)
	topicName := fmt.Sprintf("video-%s-%s", peers[0], peers[1])
	ctx := context.Background()
	cctx, cancelFunc := context.WithCancel(ctx)
	go wsLoop(ctx, cancelFunc, ws, topicName, userID)
	natsConnLoop(cctx, ctx, ws, topicName, userID)
}

const TEMPLATE = "layouts/layout.html"
const STATIC_DIR = "/static/"

type API struct {
}

type PageData struct {
	Title        string
	Content      string
	CanonicalURL string

	OGTitle       string
	OGDescription string
	OGType        string
	OGImage       string
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func makeGzipHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		h.ServeHTTP(gzr, r)
	})
}

func notFound(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles(TEMPLATE, "content/custom_404.html")
	w.WriteHeader(http.StatusNotFound)
	t.ExecuteTemplate(w, "layout", &PageData{
		Title: "DUMMY", Content: ""})
}

func fileHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "."+STATIC_DIR+r.URL.Path[1:])
}

func maxAgeHandler(seconds int, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", fmt.Sprintf("max-age=%d, public, immutable", seconds))
		h.ServeHTTP(w, r)
	})
}

func compileTemplates(ua string, filenames ...string) (*template.Template, error) {
	var tmpl *template.Template
	for _, filename := range filenames {
		name := filepath.Base(filename)
		if tmpl == nil {
			tmpl = template.New(name).Funcs(template.FuncMap{})
		} else {
			tmpl = tmpl.New(name).Funcs(template.FuncMap{})
		}

		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		tmpl.Parse(string(b))
	}
	return tmpl, nil
}

type HomePageData struct {
	*PageData
}

func (api *API) index(w http.ResponseWriter, r *http.Request) {
	ua := r.Header.Get("User-Agent")
	t, err := compileTemplates(ua, TEMPLATE, "content/index.html")
	if err != nil {
		panic(err)
	}
	err = t.ExecuteTemplate(w, "layout", &HomePageData{
		PageData: &PageData{
			CanonicalURL: "",
			Title:        "",
			Content:      "",

			OGTitle:       "",
			OGDescription: "",
			OGImage:       "",
			OGType:        "",
		}})
	if err != nil {
		panic(err)
	}
}

func main() {
	api := &API{}
	err := Initialize()
	if err != nil {
		panic(err)
	}
	router := mux.NewRouter().StrictSlash(true)
	router.
		PathPrefix("/static/").
		Handler(http.StripPrefix(STATIC_DIR, makeGzipHandler(maxAgeHandler(2629746, http.FileServer(http.Dir("."+STATIC_DIR))))))
	router.HandleFunc("/robots.txt", fileHandler).Methods("GET")
	router.HandleFunc("/", api.index).Methods("GET")
	router.NotFoundHandler = http.HandlerFunc(notFound)
	router.HandleFunc("/video/connections", VideoConnections).Methods(http.MethodGet)
	tls.LoadX509KeyPair("localhost.crt", "localhost.key")
	log.Fatal(http.ListenAndServe(":8000", router))
}
