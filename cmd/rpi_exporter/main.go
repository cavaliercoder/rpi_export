package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/cavaliercoder/rpi_export/pkg/export/prometheus"
	"github.com/cavaliercoder/rpi_export/pkg/mbox"
)

var (
	flagAddr  = flag.String("addr", "", "Listen on address")
	flagDebug = flag.Bool("debug", false, "Print debug messages")
)

func main() {
	flag.Parse()
	mbox.Debug = *flagDebug

	if *flagAddr != "" {
		http.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := prometheus.Write(w); err != nil {
				log.Printf("Error: %v", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}))
		log.Printf("Listening on %s", *flagAddr)
		http.ListenAndServe(*flagAddr, nil)
		return
	}

	if err := prometheus.Write(os.Stdout); err != nil {
		log.Fatal(err)
	}
}
