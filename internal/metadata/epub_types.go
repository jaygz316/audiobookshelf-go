package metadata

import (
	"encoding/xml"
)

// container XML structures
type container struct {
	XMLName   xml.Name  `xml:"container"`
	Rootfiles rootfiles `xml:"rootfiles"`
}

type rootfiles struct {
	Rootfile []rootfile `xml:"rootfile"`
}

type rootfile struct {
	FullPath string `xml:"full-path,attr"`
}

// opfPackage XML structures
type opfPackage struct {
	XMLName  xml.Name    `xml:"package"`
	Metadata opfMetadata `xml:"metadata"`
	Manifest opfManifest `xml:"manifest"`
	Spine    opfSpine    `xml:"spine"`
}

type opfMetadata struct {
	Title       []opfString  `xml:"title"`
	Creator     []opfCreator `xml:"creator"`
	Publisher   []string     `xml:"publisher"`
	Date        []string     `xml:"date"`
	Language    []string     `xml:"language"`
	Identifier  []opfID      `xml:"identifier"`
	Description []string     `xml:"description"`
	Meta        []opfMeta    `xml:"meta"`
}

type opfString struct {
	Value string `xml:",chardata"`
}

type opfCreator struct {
	Value  string `xml:",chardata"`
	Role   string `xml:"role,attr"`
	FileAs string `xml:"file-as,attr"`
	ID     string `xml:"id,attr"`
}

type opfID struct {
	Value  string `xml:",chardata"`
	Scheme string `xml:"scheme,attr"`
	ID     string `xml:"id,attr"`
}

type opfMeta struct {
	Name     string `xml:"name,attr"`
	Content  string `xml:"content,attr"`
	Refines  string `xml:"refines,attr"`
	Property string `xml:"property,attr"`
	Value    string `xml:",chardata"`
}

type opfManifest struct {
	Items []opfItem `xml:"item"`
}

type opfItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type opfSpine struct {
	Toc string `xml:"toc,attr"`
}

// ncx XML structures
type ncx struct {
	XMLName xml.Name  `xml:"ncx"`
	NavMap  ncxNavMap `xml:"navMap"`
}

type ncxNavMap struct {
	NavPoints []ncxNavPoint `xml:"navPoint"`
}

type ncxNavPoint struct {
	NavLabel  ncxNavLabel   `xml:"navLabel"`
	NavPoints []ncxNavPoint `xml:"navPoint"`
}

type ncxNavLabel struct {
	Text string `xml:"text"`
}
