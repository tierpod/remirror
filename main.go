package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

var (
	http_client = http.Client{}

	downloads_mu sync.Mutex
	downloads    = map[string]*Download{}
)

type Download struct {
	resp *http.Response

	tmp_path string
	tmp_size int64
	tmp_done chan struct{} // will be closed when download is done and final bytes written
}

var mirrors = map[string]string{
	"/archlinux/":   "https://mirrors.kernel.org",
	"/centos/":      "https://mirrors.xmission.com",
	"/fedora/":      "https://mirrors.xmission.com",
	"/fedora-epel/": "https://mirrors.xmission.com",
	"/experticity/": "http://yum.dev.experticity.com",
	"/java/":        "http://yum.dev.experticity.com",
	"/golang/":      "https://storage.googleapis.com",
	"/misc/":        "http://yum.dev.experticity.com",
	"/linux/chrome": "http://dl.google.com",

	// These mariadb ones are super crappy... likely to collide with something.
	// It's too bad they didn't do a nice URL prefix.
	"/5.5/centos7-amd64":  "http://yum.mariadb.org",
	"/10.2/centos7-amd64": "http://yum.mariadb.org",
	"/10.3/centos7-amd64": "http://yum.mariadb.org",
}

func should_cache(path string) bool {
	// Arch has some DB files we don't want to cache even though
	// they have archive suffixes. So we're a little more strict here.
	if strings.HasPrefix(path, "/archlinux/") {
		if strings.HasSuffix(path, ".pkg.tar.xz") {
			return true
		}
		if strings.HasSuffix(path, ".pkg.tar.xz.sig") {
			return true
		}
		return false
	}

	// Otherwise cache everything that looks like an archive.
	if strings.HasSuffix(path, ".xz") ||
		strings.HasSuffix(path, ".gz") ||
		strings.HasSuffix(path, ".bz2") ||
		strings.HasSuffix(path, ".zip") ||
		strings.HasSuffix(path, ".tgz") ||
		strings.HasSuffix(path, ".rpm") ||
		strings.HasSuffix(path, "-rpm.bin") ||
		strings.HasSuffix(path, ".xz.sig") {
		return true
	}
	return false
}

func main() {

	var (
		listen string
		data   string
		localyum string
	)

	flag.StringVar(&listen, "listen", ":8084", "HTTP listen address")
	flag.StringVar(&data, "data", "/var/remirror", "Data storage path (data in here is public)")
	flag.StringVar(&localyum, "localyum", "", "Path to local experticity yum repo for /experticity/")

	flag.Parse()

	if localyum != "" {
		http.Handle("/experticity/",
			http.FileServer(http.Dir(localyum)))
	}

	fileserver := http.FileServer(http.Dir(data))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		log.Println(r.Method + " http://" + r.Host + r.RequestURI)

		err := func() error {

			upstream := ""

			for prefix, mirror := range mirrors {
				if strings.HasPrefix(r.URL.Path, prefix) {
					upstream = mirror
				}
			}

			if upstream == "" {
				fmt.Println("no upstream found for url", r.URL.Path)
				return HTTPError(404)
			}

			local_path := ""

			if should_cache(r.URL.Path) {
				local_path = data + path.Clean(r.URL.Path)

				_, err := os.Stat(local_path)
				if err == nil {
					fileserver.ServeHTTP(w, r)
					return nil
				}
			}

			var download *Download
			var ok bool

			downloads_mu.Lock()

			if r.Header.Get("Range") == "" && local_path != "" {
				download, ok = downloads[local_path]
				if ok {
					fh, err := os.Open(download.tmp_path)
					downloads_mu.Unlock()
					if err != nil {
						return err
					}
					return tmp_download(local_path, w, download, fh)
				}
			}

			// downloads_mu is still locked. take care.
			// we need to keep it locked until we have
			// registered a download, opened a temp file,
			// and saved it's path into the tmp_path in
			// the struct.
			// then we need to make sure to release.

			log.Println("-->", upstream+r.RequestURI)

			req, err := http.NewRequest("GET", upstream+r.RequestURI, nil)
			if err != nil {
				downloads_mu.Unlock()
				return err
			}

			for k, vs := range r.Header {
				if !hopHeaders[k] {
					for _, v := range vs {
						req.Header.Add(k, v)
					}
				}
			}

			resp, err := http_client.Do(req)
			if err != nil {
				downloads_mu.Unlock()
				return err
			}
			defer resp.Body.Close()

			out := io.Writer(w)

			tmp_path := ""

			var tmp_needs_final_close io.Closer

			// We don't want to cache the result if the server
			// returns with a 206.
			if resp.StatusCode == 200 && local_path != "" {
				tmp, err := ioutil.TempFile(data, "remirror_tmp_")
				if err != nil {
					downloads_mu.Unlock()
					return err
				}
				tmp_needs_final_close = tmp
				tmp_path = tmp.Name()
				//fmt.Println("tmp", tmp_path)

				defer tmp.Close()
				defer os.Remove(tmp_path)

				out = io.MultiWriter(out, tmp)

				// don't use concurrent download struct if the response
				// doesn't have a clear content length. it's too hard
				// to tell in the other goroutines when the tmp_file reads
				// should be "done".
				if resp.ContentLength > 0 {
					// at this point we have a "successful" download in
					// progress. save into the struct.
					download = &Download{
						resp:     resp,
						tmp_path: tmp_path,
						tmp_size: resp.ContentLength,
						tmp_done: make(chan struct{}),
					}
					downloads[local_path] = download
				}
			}
			// release the mutex. if we have a successful download in
			// progress, we have stored it correctly so far. if not,
			// we unlock, leaving the download struct unmodified. the
			// next request to try that URL will retry.
			downloads_mu.Unlock()

			// however we quit, we want to clear the download in progress
			// entry. this deferred func should run before the deferred
			// cleanup funcs above, so the filehandle should still be
			// valid when we clear it out.
			defer func() {
				if download == nil {
					// we didn't end up using the map for some reason.
					// (maybe empty content length, non 200 response, etc)
					return
				}

				// make sure final close has been called. things might still
				// be writing, and we need that to be done before
				// we close tmp_done
				_ = tmp_needs_final_close.Close()

				close(download.tmp_done)

				downloads_mu.Lock()
				delete(downloads, local_path)
				downloads_mu.Unlock()
			}()

			write_resp_headers(w, resp)

			n, err := io.Copy(out, resp.Body)
			if err != nil {
				log.Println(err)
				return nil
			}

			if n != resp.ContentLength {
				if resp.ContentLength != -1 {
					log.Printf("Short data returned from server (Content-Length %d received %d)\n", resp.ContentLength, n)
				}
				// Not really an HTTP error, leave it up to the client.
				// but we aren't going to save our response to the cache.
				return nil
			}

			if tmp_path != "" {
				os.MkdirAll(path.Dir(local_path), 0755)

				err = tmp_needs_final_close.Close()
				if err != nil {
					log.Println(err)
					return nil
				}

				// clear from struct before renaming
				if download != nil {
					close(download.tmp_done)
					downloads_mu.Lock()
					delete(downloads, local_path)
					downloads_mu.Unlock()
					download = nil // so we don't re-close
				}

				err = os.Rename(tmp_path, local_path)
				if err != nil {
					log.Println(err)
					return nil
				}
				log.Println(">:)")
			}

			return nil
		}()

		he, ok := err.(HTTPError)
		if ok {
			http.Error(w, he.Error(), he.Code())
			fmt.Println("\t\t", he.Error())
		} else if err != nil {
			http.Error(w, err.Error(), 500)
			fmt.Println("\t\t500 " + err.Error())
		}
	})

	log.Println("arch/fedora/centos/experticity mirror proxy listening on HTTP " + listen)
	log.Fatal(http.ListenAndServe(listen, nil))
}

func write_resp_headers(w http.ResponseWriter, resp *http.Response) {

	for k, vs := range resp.Header {
		if k == "Accept-Ranges" {
			continue
		}
		for _, v := range vs {
			//fmt.Printf("proxy back header %#v\t%#v\n", k, v)
			w.Header().Add(k, v)
		}
	}

	w.Header().Set("Server", "remirror")
	w.WriteHeader(resp.StatusCode)
}

// return a download in progress started by another request
func tmp_download(local_path string, w http.ResponseWriter, download *Download, tmp io.ReadCloser) error {
	defer tmp.Close()

	write_resp_headers(w, download.resp)

	written := int64(0)
	done := false
	last := time.Now()

	for {
		n, err := io.Copy(w, tmp)

		if n < 0 {
			panic(fmt.Sprintf("io.Copy returned n %d: Not what I expected!", n))
		}

		written += n

		if err != nil && err != io.EOF {
			log.Printf("Error while reading concurrent download %#s from %#s: %v\n",
				local_path, download.tmp_path, err)
			// Not an HTTP error: just return, and the client will hopefully
			// handle a short read correctly.
			return nil
		}

		if n > 0 {
			// cool, try another copy. hopefully the file
			// has more bytes now
			last = time.Now()
			continue
		}

		if done {
			return nil
		}

		// sleep for a bit so the other download has a chance to write
		// more bytes.
		select {
		case <-time.After(time.Second):
			// 60 second timeout for the other goroutine to at least write _something_
			if time.Since(last) > time.Minute {
				log.Println("Timeout while reading concurrent download %#s from %#s\n",
					local_path,
					download.tmp_path)
				// Not an HTTP error: just return, and the client will hopefully
				// handle a short read correctly.
				return nil
			}
			continue
		case <-download.tmp_done:
			done = true
			continue
		}
	}
}
