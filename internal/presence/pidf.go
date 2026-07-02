package presence

import "fmt"

func pidfXML(entity, tupleID, basic, note string) []byte {
	if note == "" {
		note = basic
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<presence xmlns="urn:ietf:params:xml:ns:pidf" entity="%s">
  <tuple id="%s">
    <status><basic>%s</basic></status>
    <note>%s</note>
  </tuple>
</presence>`, entity, tupleID, basic, note))
}

func entityURI(ext, domain string) string {
	return fmt.Sprintf("sip:%s@%s", ext, domain)
}
