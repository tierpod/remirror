package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"io"
	"os"
	"strconv"
	"io/ioutil"
	"log"
	"net/http"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"path"
	"regexp"
	"strings"
	"time"
)

var (
	http_client = http.Client{}
	dns_client  = dns.Client{}

	re_revision = regexp.MustCompile(`<revision>(\d+)</revision>`)
	re_timestamp = regexp.MustCompile(`<timestamp>(\d+)</timestamp>`)
)

type HTTPError int

func (e HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d %s", e, http.StatusText(e.Code()))
}
func (e HTTPError) Code() int {
	return int(e)
}

func should_cache(path string) bool {
	if strings.HasSuffix(path, ".pkg.tar.xz") {
		return true
	}
	if strings.HasSuffix(path, ".rpm") {
		return true
	}
	if strings.Contains(path, "/repodata/") && (strings.HasSuffix(path, ".gz") ||
		strings.HasSuffix(path, ".bz2") || strings.HasSuffix(path, ".xz")) {
		return true
	}
//	if strings.HasPrefix(path, "/our_private_repomdcache/") {
//		return true
//	}
	return false
}

func main() {

	var (
		listen     string
		dns_server string
		data       string
		host       string
	)

	flag.StringVar(&listen, "listen", ":80", "HTTP listen address")
	flag.StringVar(&dns_server, "dns", "8.8.8.8", "DNS server to use for man in the middle mirrorlist interception")
	flag.StringVar(&data, "data", "/var/remirror", "Data storage path (data in here is public)")
	flag.StringVar(&host, "host", "9ex-dc-mirror", "This hosts name, so we can return a mirrorlist with ourselves")

	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		log.Println(r.Method + " http://" + r.Host + r.RequestURI)


		err := func() error {

			// Some special sauce mirrorlist handlers that will point to ourselves
			if r.Host == "mirrors.fedoraproject.org" {
				return fedora_mirrorlist(w, r, dns_server, host, data)
			}
			if r.Host == "mirrorlist.centos.org" {
				return centos_mirrorlist(w, r, dns_server, host)
			}

			// Now we guess the upstream from the URL
			upstream := ""

			if strings.HasPrefix(r.URL.Path, "/archlinux/") {
				upstream = "https://mirrors.kernel.org"
			} else if strings.HasPrefix(r.URL.Path, "/centos/") {
				upstream = "https://mirrors.xmission.com"
			} else if strings.HasPrefix(r.URL.Path, "/fedora/") {
				upstream = "https://mirrors.xmission.com"
			} else if strings.HasPrefix(r.URL.Path, "/fedora-epel/") {
				upstream = "https://mirrors.xmission.com"
			} else if strings.HasPrefix(r.URL.Path, "/our_private_repomdcache/") {
				upstream = "wtf"
			}

			if upstream == "" {
				fmt.Println("no upstream found for url", r.URL.Path)
				return HTTPError(404)
			}

			local_path := ""

			if should_cache(r.URL.Path) {
				local_path = data + path.Clean(r.URL.Path)

				stat, err := os.Stat(local_path)
				if err == nil {
					fh, err := os.Open(local_path)
					if err != nil {
						return err
					}
					defer fh.Close()

					w.Header().Set("Content-Length", strconv.Itoa(int(stat.Size())))
					w.Header().Set("Server", "remirror")
					_, err = io.Copy(w, fh)
					if err != nil {
						log.Println(err)
					}
					return nil
				}
			}

			if upstream == "wtf" {
				return HTTPError(404)
			}

			log.Println("-->", upstream+r.RequestURI)

			req, err := http.NewRequest("GET", upstream+r.RequestURI, nil)
			if err != nil {
				return err
			}

			for k, vs := range r.Header {
				if k != "Host" {
					for _, v := range vs {
						req.Header.Add(k, v)
					}
				}
			}

			resp, err := http_client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			out := io.Writer(w)

			tmp_path := ""

			if resp.StatusCode == 200 && local_path != "" {
				tmp, err := ioutil.TempFile(data, "remirror_tmp_")
				if err != nil {
					return err
				}
				tmp_path = tmp.Name()
				//fmt.Println("tmp", tmp_path)

				defer tmp.Close()
				defer os.Remove(tmp_path)

				out = io.MultiWriter(out, tmp)
			}

			for k, vs := range resp.Header {
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}

			w.Header().Set("Server", "remirror")
			w.Header().Set("Content-Length", strconv.Itoa(int(resp.ContentLength)))

			w.WriteHeader(resp.StatusCode)

			_, err = io.Copy(out, resp.Body)
			if err != nil {
				log.Println(err)
				return nil
			}

			if tmp_path != "" {
				os.MkdirAll(path.Dir(local_path), 0755)

				err = os.Rename(tmp_path, local_path)
				if err != nil {
					log.Println(err)
					return nil
				}
			}

			return nil
		}()

		he, ok := err.(HTTPError)
		if ok {
			http.Error(w, he.Error(), he.Code())
			fmt.Println("\t\t", he.Error())
		} else if err != nil {
			http.Error(w, err.Error(), 500)
			fmt.Println("\t\t500 "+err.Error())
		}
	})

	log.Println("arch/fedora/centos mirror proxy listening on HTTP " + listen)
	log.Fatal(http.ListenAndServe(listen, nil))
}

func centos_mirrorlist(w http.ResponseWriter, r *http.Request, dns_server, host string) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	release := r.Form.Get("release")
	repo := r.Form.Get("repo")
	arch := r.Form.Get("arch")

	if release == "7" {
		release = "7.2.1511"
	}

	w.Header().Set("Content-Type", "text/plain")

	us := "http://" + host + "/centos/" + release + "/" + repo + "/" + arch + "/"

	io.WriteString(w, us)
	return nil
}

func fedora_mirrorlist(w http.ResponseWriter, r *http.Request, dns_server, host, data string) error {

	//fmt.Println("fedora_mirrorlist", r.URL.String())

	err := r.ParseForm()
	if err != nil {
		return err
	}

	repo := r.Form.Get("repo")
	arch := r.Form.Get("arch")

	if repo != "epel-7" {
		fmt.Println("not sure how to handle fedora repo", repo)
		return HTTPError(404)
	}

	upstream := "http://mirror.oss.ou.edu/epel/7/" + arch + "/repodata/repomd.xml"

	resp, err := http_client.Get(upstream)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return HTTPError(resp.StatusCode)
	}

	tmp, err := ioutil.TempFile(data, "remirror_tmp_")
	if err != nil {
		return err
	}
	tmp_path := tmp.Name()
	//fmt.Println("tmp", tmp_path)

	defer tmp.Close()
	defer func() {
		if tmp_path != "" {
			os.Remove(tmp_path)
		}
	}()

	md5_sum := md5.New()
	sha1_sum := sha1.New()
	sha256_sum := sha256.New()
	sha512_sum := sha512.New()
	buf := &bytes.Buffer{}

	out := io.MultiWriter(
		tmp,
		md5_sum,
		sha1_sum,
		sha256_sum,
		sha512_sum,
		buf,
	)

	n, err := io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	// http://mirrors.kernel.org/fedora-epel/7/x86_64/repodata/repomd.xml

	//local_url := "/our_private_repomdcache/" + fmt.Sprintf("%x", md5_sum.Sum(nil)) + "/epel/7/" + arch + "/repodata/repomd.xml"

	local_url := "/fedora-epel/7/" + arch + "/repodata/repomd.xml"
	local_path := data + local_url

	os.MkdirAll(path.Dir(local_path), 0755)

	err = os.Rename(tmp_path, local_path)
	if err != nil {
		return err
	}
	tmp_path = ""

	w.Header().Set("Content-Type", "application/metalink+xml")

	ts_matches := re_timestamp.FindAllSubmatch(buf.Bytes(), -1)
	if ts_matches == nil {
		return fmt.Errorf("no <timestamp> tag found in repomd.xml")
	}

	ts_txt := ""
	for _, ts_m := range ts_matches {
		ts_txt = string(ts_m[1])
	}

	// apparently the validation is retarded in yum and
	// looks at the "last" timestamp

	matches := re_revision.FindSubmatch(buf.Bytes())
	if matches == nil {
		return fmt.Errorf("no <revision> tag found in repomd.xml")
	}

	revision_txt := string(matches[1])

	revision_txt = ts_txt

	//fmt.Println("extracted timestamp", revision_txt)

	revision, err := strconv.Atoi(revision_txt)
	if err != nil {
		return err
	}

	now := time.Unix(int64(revision), 0)
	txt := now.Format(time.RFC1123)

	us := "http://" + host + local_url

	_, err = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?>
<metalink version="3.0" xmlns="http://www.metalinker.org/" type="dynamic" pubdate="`+txt+`" generator="mirrormanager" xmlns:mm0="http://fedorahosted.org/mirrormanager">
  <files>
    <file name="repomd.xml">
      <mm0:timestamp>`+revision_txt+`</mm0:timestamp>
      <size>`+strconv.Itoa(int(n))+`</size>
      <verification>
        <hash type="md5">`+fmt.Sprintf("%x", md5_sum.Sum(nil))+`</hash>
        <hash type="sha1">`+fmt.Sprintf("%x", sha1_sum.Sum(nil))+`</hash>
        <hash type="sha256">`+fmt.Sprintf("%x", sha256_sum.Sum(nil))+`</hash>
        <hash type="sha512">`+fmt.Sprintf("%x", sha512_sum.Sum(nil))+`</hash>
      </verification>
      <resources maxconnections="1">
        <url protocol="http" type="http" location="US" preference="100">`+us+`</url>
      </resources>
    </file>
  </files>
</metalink>
`)

	if err != nil {
		log.Println(err)
	}

	return nil
}

func resolve(host, dns_server string) (string, error) {
	m := dns.Msg{}
	m.SetQuestion(host+".", dns.TypeA)
	dnsr, _, err := dns_client.Exchange(&m, dns_server+":53")
	if err != nil {
		return "", err
	}
	for _, ans := range dnsr.Answer {
		a, a_ok := ans.(*dns.A)
		if a_ok {
			return a.String(), nil
		}
	}
	return "", fmt.Errorf("Server not found (%#v, queried DNS server %#v)", host, dns_server)
}
