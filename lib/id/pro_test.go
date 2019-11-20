package id

import (
	"philosopher/test"
	"strings"
	"testing"
)

func TestProtXML_MarkUniquePeptides(t *testing.T) {

	test.SetupTestEnv()

	var p ProtXML

	p.Read("interact.prot.xml")
	p.DecoyTag = "rev_"

	var unique int
	var flag bool

	type fields struct {
		FileName   string
		DecoyTag   string
		Groups     GroupList
		RunOptions string
	}
	type args struct {
		w float64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "Testing protein Unique marking.",
			args: args{w: 1.00},
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {

			p.MarkUniquePeptides(tt.args.w)

			p.PromoteProteinIDs()

			for _, i := range p.Groups {
				for _, j := range i.Proteins {

					if strings.HasPrefix(string(j.ProteinName), p.DecoyTag) {
						for _, k := range j.IndistinguishableProtein {
							if !strings.HasPrefix(string(k), p.DecoyTag) {
								flag = true
							}
						}
					}

					for _, k := range j.PeptideIons {

						if k.IsUnique == true {
							unique++
						}

					}
				}
			}

			if unique != 38412 {
				t.Errorf("Number of Unque ions in ProtXML is wrong, got %v, want %v", unique, 38412)
			}

			if flag == true {
				t.Errorf("Protein Promotion is no working properly")
			}

		})
	}

	test.ShutDowTestEnv()
}