package router

import (
	"strconv"
	"strings"
)

type Kind int

const (
	KindUnknown Kind = iota
	KindExtension
	KindHunt
	KindConference
	KindPaging
	KindTrunk
	KindRedial
	KindCallReturn
	KindTransfer
	KindPark
	KindParkRetrieve
	KindDNDActivate
	KindDNDDeactivate
)

type Route struct {
	Kind   Kind
	Target string
	Prefix string
	Rest   string
}

// FeatureCodes holds configurable star-code dial strings.
type FeatureCodes struct {
	Redial        string
	CallReturn    string
	Transfer      string
	Park          string
	ParkRetrieve  string
	DNDActivate   string
	DNDDeactivate string
}

// DefaultFeatureCodes returns built-in star code defaults.
func DefaultFeatureCodes() FeatureCodes {
	return FeatureCodes{
		Redial:        "*66",
		CallReturn:    "*69",
		Transfer:      "*77",
		Park:          "*85",
		ParkRetrieve:  "*86",
		DNDActivate:   "*78",
		DNDDeactivate: "*79",
	}
}

func RouteDial(dial string, fc FeatureCodes) Route {
	dial = strings.TrimSpace(dial)
	if dial == "" {
		return Route{Kind: KindUnknown}
	}

	if strings.HasPrefix(dial, "*") {
		switch dial {
		case fc.Redial:
			return Route{Kind: KindRedial}
		case fc.CallReturn:
			return Route{Kind: KindCallReturn}
		case fc.Transfer:
			return Route{Kind: KindTransfer}
		case fc.Park:
			return Route{Kind: KindPark}
		case fc.ParkRetrieve:
			return Route{Kind: KindParkRetrieve}
		case fc.DNDActivate:
			return Route{Kind: KindDNDActivate}
		case fc.DNDDeactivate:
			return Route{Kind: KindDNDDeactivate}
		}
		if fc.ParkRetrieve != "" && strings.HasPrefix(dial, fc.ParkRetrieve) && len(dial) > len(fc.ParkRetrieve) {
			slot := strings.TrimPrefix(dial, fc.ParkRetrieve)
			if slot != "" {
				return Route{Kind: KindParkRetrieve, Target: slot}
			}
		}
		code := strings.TrimPrefix(dial, "*")
		if n, err := strconv.Atoi(code); err == nil && n >= 80 && n <= 99 {
			return Route{Kind: KindPaging, Target: code}
		}
		return Route{Kind: KindPaging, Target: code}
	}

	if len(dial) >= 2 {
		p2 := dial[:2]
		if p2 >= "90" && p2 <= "99" && len(dial) > 2 {
			return Route{Kind: KindTrunk, Prefix: p2, Rest: dial[2:], Target: dial}
		}
	}

	if n, err := strconv.Atoi(dial); err == nil {
		if n >= 100 && n <= 499 {
			return Route{Kind: KindExtension, Target: dial}
		}
		if n >= 500 && n <= 599 {
			return Route{Kind: KindHunt, Target: dial}
		}
		if n >= 600 && n <= 699 {
			return Route{Kind: KindConference, Target: dial}
		}
	}

	return Route{Kind: KindUnknown, Target: dial}
}
