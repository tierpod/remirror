package main

import (
	"encoding/xml"
	"strconv"
)

type Metalink struct {
	XMLName xml.Name `xml:"metalink"`

	Version   string `xml:"version,attr"`
	XMLns     string `xml:"xmlns,attr"`
	Type      string `xml:"type,attr"`
	PubDate   string `xml:"pubdate,attr"`
	Generator string `xml:"generator,attr"`

	Text  string `xml:",chardata"`
	Files Files
}

type Files struct {
	XMLName xml.Name    `xml:"files"`
	Text    string      `xml:",chardata"`
	File    []*MetaFile `xml:"file"`
}

type MetaFile struct {
	XMLName      xml.Name          `xml:"file"`
	Name         string            `xml:"name,attr"`
	Text         string            `xml:",chardata"`
	Size         Size              `xml:"size"`
	Resources    []*Resources      `xml:"resources"`
	Verification *FileVerification `xml:"verification"`
}

type FileVerification struct {
	XMLName xml.Name `xml:"verification"`
	Hashes  []*Hash  `xml:"hash"`
}

type Hash struct {
	XMLName xml.Name `xml:"hash"`
	Type    string   `xml:"type,attr"`
	Text    string   `xml:",chardata"`
}

type Size struct {
	XMLName xml.Name `xml:"size"`
	Text    string   `xml:",chardata"`
}

type Resources struct {
	XMLName xml.Name `xml:"resources"`
	Urls    []*Url   `xml:"url"`
}

type Url struct {
	XMLName    xml.Name `xml:"url"`
	Type       string   `xml:"type,attr"`
	Protocol   string   `xml:"protocol,attr"`
	Location   string   `xml:"location,attr"`
	Preference string   `xml:"preference,attr"`
	Link       string   `xml:",chardata"`
}

func parse_and_munge_metalink(input []byte, add_mirror string) ([]byte, error) {

	// just to make the output pretty
	for i, c := range input {
		if c == '\r' || c == '\n' {
			input[i] = ' '
		}
	}

	ml := Metalink{}
	err := xml.Unmarshal(input, &ml)
	if err != nil {
		return nil, err
	}

	for _, file := range ml.Files.File {
		for _, resource := range file.Resources {
			for _, url := range resource.Urls {
				i, err := strconv.Atoi(url.Preference)
				if err != nil {
					return nil, err
				}

				// we want to be #100 :)
				if i > 1 {
					i--
				}

				url.Preference = strconv.Itoa(i)
			}

			// Prepend our entry to the top of the list. In theory the consumer should sort by
			// preference so this isn't needed, but I'm not sure if everything does it that way.
			//				resource.Urls = append([]*Url{
			//				}, resource.Urls...)

			resource.Urls = append(resource.Urls, &Url{
				Type:       "http",
				Protocol:   "http",
				Location:   "US",
				Preference: "100",
				Link:       add_mirror,
			})
		}
	}

	return xml.MarshalIndent(&ml, "", "  ")
}
