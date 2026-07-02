package call

import (
	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
)

func testServerDialog(id string) *diago.DialogServerSession {
	return &diago.DialogServerSession{
		DialogServerSession: &sipgo.DialogServerSession{
			Dialog: sipgo.Dialog{ID: id},
		},
	}
}
