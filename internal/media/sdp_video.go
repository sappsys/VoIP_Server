package media

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/emiago/diago/media/sdp"
)

// VideoInfo describes an H.264 (or other) video m-line from SDP.
type VideoInfo struct {
	PayloadTypes []int
	RemoteAddr   *net.UDPAddr
	Attributes   []string
	Proto        string
}

// HasVideo returns true when SDP contains a video media line.
func HasVideo(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	sd := sdp.SessionDescription{}
	if sdp.Unmarshal(body, &sd) != nil {
		return false
	}
	for _, line := range sd.Values("m") {
		if strings.HasPrefix(line, "video ") {
			return true
		}
	}
	return false
}

// ParseVideo extracts video endpoint details from SDP.
func ParseVideo(body []byte) (*VideoInfo, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty sdp")
	}
	sd := sdp.SessionDescription{}
	if err := sdp.Unmarshal(body, &sd); err != nil {
		return nil, err
	}
	var md sdp.MediaDescription
	var mdLine string
	for _, line := range sd.Values("m") {
		if strings.HasPrefix(line, "video ") {
			mdLine = line
			break
		}
	}
	if mdLine == "" {
		return nil, fmt.Errorf("Media not found for %q", "video")
	}
	fields := strings.Fields(mdLine)
	if len(fields) < 4 {
		return nil, fmt.Errorf("invalid video m-line")
	}
	md.MediaType = fields[0]
	ports := strings.Split(fields[1], "/")
	md.Port, _ = strconv.Atoi(ports[0])
	md.Proto = fields[2]
	md.Formats = fields[3:]

	conn, err := sd.ConnectionInformation()
	if err != nil {
		return nil, err
	}
	port := md.Port
	for _, attr := range sd.Values("a") {
		if strings.HasPrefix(attr, "rtcp:") {
			if p, err := strconv.Atoi(strings.TrimPrefix(attr, "rtcp:")); err == nil && p > 0 {
				port = p
			}
		}
	}
	pts := make([]int, 0, len(md.Formats))
	for _, f := range md.Formats {
		if n, err := strconv.Atoi(f); err == nil {
			pts = append(pts, n)
		}
	}
	var attrs []string
	for _, a := range sd.Values("a") {
		if strings.HasPrefix(a, "rtpmap:") || strings.HasPrefix(a, "fmtp:") ||
			strings.HasPrefix(a, "rtcp-fb:") || a == "sendrecv" || a == "sendonly" ||
			a == "recvonly" || a == "inactive" {
			attrs = append(attrs, a)
		}
	}
	return &VideoInfo{
		PayloadTypes: pts,
		RemoteAddr:   &net.UDPAddr{IP: conn.IP, Port: port},
		Attributes:   attrs,
		Proto:        md.Proto,
	}, nil
}

// AppendVideo merges a video m-line into an audio SDP answer, reusing codecs from the offer.
func AppendVideo(audioSDP, offerSDP []byte, localIP string, localPort int) ([]byte, error) {
	vOffer, err := ParseVideo(offerSDP)
	if err != nil {
		return nil, err
	}
	proto := vOffer.Proto
	if proto == "" {
		proto = "RTP/AVP"
	}
	formats := make([]string, len(vOffer.PayloadTypes))
	for i, p := range vOffer.PayloadTypes {
		formats[i] = strconv.Itoa(p)
	}
	if len(formats) == 0 {
		formats = []string{"96"}
	}
	lines := strings.Split(strings.TrimSuffix(string(audioSDP), "\r\n"), "\n")
	var out []string
	inserted := false
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		out = append(out, line)
		if !inserted && strings.HasPrefix(line, "m=audio ") {
			out = append(out,
				fmt.Sprintf("m=video %d %s %s", localPort, proto, strings.Join(formats, " ")),
				fmt.Sprintf("c=IN IP4 %s", localIP),
			)
			for _, a := range vOffer.Attributes {
				out = append(out, "a="+a)
			}
			out = append(out, "a=sendrecv")
			inserted = true
		}
	}
	if !inserted {
		return nil, fmt.Errorf("audio m-line not found")
	}
	return []byte(strings.Join(out, "\r\n") + "\r\n"), nil
}
