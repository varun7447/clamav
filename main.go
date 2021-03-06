/*
   Copyright 2017 Mike Lloyd

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/allegro/bigcache"
	"github.com/cloudfoundry-community/go-cfenv"
	"gopkg.in/robfig/cron.v2"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultPort = 8080
)

func init() {
	// TODO add runtime.Caller(1) info to it.
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
}

func main() {
	var port string

	// this logic just feels weird to me. idk.
	appEnv, err := cfenv.Current()
	if err != nil {
		log.Error(err)
		port = fmt.Sprintf(":%d", defaultPort)
	} else {
		port = fmt.Sprintf(":%d", appEnv.Port)
	}

	// overkill, but it's a sane library.
	// we're going to cache the AV definition files.
	cache, err := bigcache.NewBigCache(bigcache.Config{
		MaxEntrySize:       500,
		Shards:             1024,
		LifeWindow:         time.Hour * 3,
		MaxEntriesInWindow: 1000 * 10 * 60,
		Verbose:            true,
		HardMaxCacheSize:   0,
	})

	if err != nil {
		log.Errorf("cannot initialise cache. %s", err)
	}

	// let the initial seed run in the background so the web server can start.
	log.Info("starting initial seed in the background.")
	dl := NewDownloader(true)
	dl.DownloadDatabase(cache)

	// start a new crontab asynchronously.
	c := cron.New()
	c.AddFunc("@hourly", func() { NewDownloader(true).DownloadDatabase(cache) })
	c.Start()

	log.Info("started cron job for definition updates.")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cacheHandler(w, r, cache)
	})

	log.Fatal(http.ListenAndServe(port, nil))
}

// cacheHandler is just a standard handler, but returns stuff from cache.
func cacheHandler(w http.ResponseWriter, r *http.Request, c *bigcache.BigCache) {
	filename := r.URL.Path[1:]

	// logs from the gorouter.
	if strings.Contains(filename, "cloudfoundry") {
		log.Warn("nothing to see here, move along.")
		http.NotFound(w, r)
	}

	entry, err := c.Get(filename)
	if err != nil {
		log.WithFields(log.Fields{
			"err":      err,
			"filename": filename,
		}).Error("cannot return cached file!")
		log.Error(err)
		http.NotFound(w, r)
	}

	log.WithFields(log.Fields{
		"filename": filename,
	}).Info("found!")
	w.Write(entry)
}
