package xml

import (
	"encoding/gob"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/gonum/plot"
	"github.com/gonum/plot/plotter"
	"github.com/gonum/plot/plotutil"
	"github.com/gonum/plot/vg"
	"github.com/prvst/cmsl-source/data/pep"
	"github.com/prvst/cmsl-source/err"
	"github.com/prvst/philosopher-source/lib/sys"
)

// PepXML data
type PepXML struct {
	FileName              string
	SpectraFile           string
	DecoyTag              string
	Database              string
	Prophet               string
	DefinedModMassDiff    map[float64]float64
	DefinedModAminoAcid   map[float64]string
	Models                []pep.DistributionPoint
	PeptideIdentification PepIDList
}

// PeptideIdentification struct
type PeptideIdentification struct {
	Index                uint32
	Spectrum             string
	Scan                 int
	Peptide              string
	Protein              string
	ModifiedPeptide      string
	AlternativeProteins  []string
	ModPositions         []uint16
	AssignedModMasses    []float64
	AssignedMassDiffs    []float64
	AssumedCharge        uint8
	HitRank              uint8
	PrecursorNeutralMass float64
	PrecursorExpMass     float64
	RetentionTime        float64
	CalcNeutralPepMass   float64
	RawMassDiff          float64
	Massdiff             float64
	Probability          float64
	Expectation          float64
	Xcorr                float64
	DeltaCN              float64
	SpRank               float64
	DiscriminantValue    float64
	ModNtermMass         float64
	Intensity            float64
}

// PepIDList is a list of PeptideSpectrumMatch
type PepIDList []PeptideIdentification

// Len function for Sort
func (p PepIDList) Len() int {
	return len(p)
}

// Less function for Sort
func (p PepIDList) Less(i, j int) bool {
	return p[i].Probability > p[j].Probability
}

// Swap function for Sort
func (p PepIDList) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// Read is the main function for parsing pepxml data
func (p *PepXML) Read(f string) error {

	var xml pep.XML
	e := xml.Parse(f)
	if e != nil {
		return e
	}

	var mpa = xml.MsmsPipelineAnalysis

	if len(mpa.AnalysisSummary) > 0 {

		p.FileName = f
		p.Database = string(mpa.MsmsRunSummary.SearchSummary.SearchDatabase.LocalPath)
		p.SpectraFile = fmt.Sprintf("%s%s", mpa.MsmsRunSummary.BaseName, mpa.MsmsRunSummary.RawData)

		var models []pep.DistributionPoint
		pps := mpa.AnalysisSummary[0].PeptideprophetSummary

		// collect distribution points from meta
		for i := range pps.DistributionPoint {
			var m pep.DistributionPoint
			m.Fvalue = pps.DistributionPoint[i].Fvalue
			m.Obs1Distr = pps.DistributionPoint[i].Obs1Distr
			m.Model1PosDistr = pps.DistributionPoint[i].Model1PosDistr
			m.Model1NegDistr = pps.DistributionPoint[i].Model1NegDistr
			m.Obs2Distr = pps.DistributionPoint[i].Obs2Distr
			m.Model2PosDistr = pps.DistributionPoint[i].Model2PosDistr
			m.Model2NegDistr = pps.DistributionPoint[i].Model2NegDistr
			m.Obs3Distr = pps.DistributionPoint[i].Obs3Distr
			m.Model3PosDistr = pps.DistributionPoint[i].Model3PosDistr
			m.Model3NegDistr = pps.DistributionPoint[i].Model3NegDistr
			m.Obs4Distr = pps.DistributionPoint[i].Obs4Distr
			m.Model4PosDistr = pps.DistributionPoint[i].Model4PosDistr
			m.Model4NegDistr = pps.DistributionPoint[i].Model4NegDistr
			m.Obs5Distr = pps.DistributionPoint[i].Obs5Distr
			m.Model5PosDistr = pps.DistributionPoint[i].Model5PosDistr
			m.Model5NegDistr = pps.DistributionPoint[i].Model5NegDistr
			m.Obs6Distr = pps.DistributionPoint[i].Obs6Distr
			m.Model6PosDistr = pps.DistributionPoint[i].Model6PosDistr
			m.Model6NegDistr = pps.DistributionPoint[i].Model6NegDistr
			m.Obs7Distr = pps.DistributionPoint[i].Obs7Distr
			m.Model7PosDistr = pps.DistributionPoint[i].Model7PosDistr
			m.Model7NegDistr = pps.DistributionPoint[i].Model7NegDistr
			models = append(models, m)
		}

		// collect variable and fixed modifications
		if len(p.DefinedModMassDiff) == 0 {
			p.DefinedModMassDiff = make(map[float64]float64)
			p.DefinedModAminoAcid = make(map[float64]string)
		}

		for i := range mpa.MsmsRunSummary.SearchSummary.AminoAcidModifications {
			p.DefinedModMassDiff[mpa.MsmsRunSummary.SearchSummary.AminoAcidModifications[i].Mass] = mpa.MsmsRunSummary.SearchSummary.AminoAcidModifications[i].MassDiff
			p.DefinedModAminoAcid[mpa.MsmsRunSummary.SearchSummary.AminoAcidModifications[i].Mass] = string(mpa.MsmsRunSummary.SearchSummary.AminoAcidModifications[i].AminoAcid)
		}

		// start processing spectra queries
		var psmlist PepIDList
		sq := mpa.MsmsRunSummary.SpectrumQuery
		for i := range sq {
			psm := processSpectrumQuery(sq[i], p.DefinedModMassDiff)
			psmlist = append(psmlist, psm)
		}

		if len(psmlist) == 0 {
			return &err.Error{Type: err.NoPSMFound, Class: err.FATA}
		}

		p.PeptideIdentification = psmlist
		p.Prophet = string(mpa.AnalysisSummary[0].Analysis)
		p.Models = models

		p.adjustMassDeviation()

		if len(psmlist) == 0 {
			logrus.Error("No PSM detected, check your files and try agains")
		}

	}

	return nil
}

func processSpectrumQuery(sq pep.SpectrumQuery, definedModMassDiff map[float64]float64) PeptideIdentification {

	var psm PeptideIdentification

	psm.Index = sq.Index
	psm.Spectrum = string(sq.Spectrum)
	psm.Scan = sq.StartScan
	psm.PrecursorNeutralMass = sq.PrecursorNeutralMass
	psm.AssumedCharge = sq.AssumedCharge
	psm.RetentionTime = sq.RetentionTimeSec

	for j := range sq.SearchResult.SearchHit {

		psm.HitRank = sq.SearchResult.SearchHit[j].HitRank
		psm.Peptide = string(sq.SearchResult.SearchHit[j].Peptide)
		psm.Protein = string(sq.SearchResult.SearchHit[j].Protein)
		psm.CalcNeutralPepMass = sq.SearchResult.SearchHit[j].CalcNeutralPepMass
		psm.Massdiff = sq.SearchResult.SearchHit[j].Massdiff

		if sq.SearchResult.SearchHit[0].AnalysisResult.InterProphetResult.Probability > 0 {
			psm.Probability = sq.SearchResult.SearchHit[0].AnalysisResult.InterProphetResult.Probability
		} else {
			psm.Probability = sq.SearchResult.SearchHit[0].AnalysisResult.PeptideProphetResult.Probability
		}

		psm.ModifiedPeptide = string(sq.SearchResult.SearchHit[j].ModificationInfo.ModifiedPeptide)
		psm.ModNtermMass = sq.SearchResult.SearchHit[j].ModificationInfo.ModNTermMass

		for k := range sq.SearchResult.SearchHit[j].AlternativeProteins {
			psm.AlternativeProteins = append(psm.AlternativeProteins, string(sq.SearchResult.SearchHit[j].AlternativeProteins[k].Protein))
		}

		for l := range sq.SearchResult.SearchHit[j].Score {
			if string(sq.SearchResult.SearchHit[j].Score[l].Name) == "expect" {
				psm.Expectation = sq.SearchResult.SearchHit[j].Score[l].Value
			} else if string(sq.SearchResult.SearchHit[j].Score[l].Name) == "xcorr" {
				psm.Xcorr = sq.SearchResult.SearchHit[j].Score[l].Value
			} else if string(sq.SearchResult.SearchHit[j].Score[l].Name) == "deltacn" {
				psm.DeltaCN = sq.SearchResult.SearchHit[j].Score[l].Value
			} else if string(sq.SearchResult.SearchHit[j].Score[l].Name) == "sprank" {
				psm.SpRank = sq.SearchResult.SearchHit[j].Score[l].Value
			}
		}

		if len(string(sq.SearchResult.SearchHit[j].ModificationInfo.ModifiedPeptide)) > 0 {
			for n := range sq.SearchResult.SearchHit[j].ModificationInfo.ModAminoacidMass {
				psm.ModPositions = append(psm.ModPositions, sq.SearchResult.SearchHit[j].ModificationInfo.ModAminoacidMass[n].Position)
				psm.AssignedModMasses = append(psm.AssignedModMasses, sq.SearchResult.SearchHit[j].ModificationInfo.ModAminoacidMass[n].Mass)
				psm.AssignedMassDiffs = append(psm.AssignedMassDiffs, definedModMassDiff[sq.SearchResult.SearchHit[j].ModificationInfo.ModAminoacidMass[n].Mass])
			}

		}
	}

	return psm
}

// adjustMassDeviation calculates the mass deviation for a pepXML file based on the 0 mass difference
func (p *PepXML) adjustMassDeviation() {

	var countZero int
	var massZero float64
	var adjustedMass float64

	for _, i := range p.PeptideIdentification {
		if math.Abs(i.Massdiff) >= -0.1 && math.Abs(i.Massdiff) <= 0.1 {
			countZero++
			massZero += i.Massdiff
		}
	}

	adjustedMass = massZero / float64(countZero)

	// keep the original massdiff on the raw variable just in case
	for i := range p.PeptideIdentification {
		p.PeptideIdentification[i].RawMassDiff = p.PeptideIdentification[i].Massdiff
		p.PeptideIdentification[i].Massdiff = (p.PeptideIdentification[i].Massdiff - adjustedMass)
	}

	return
}

// ReportModels ...
func (p *PepXML) ReportModels(session, name string) (err error) {

	var xAxis []float64

	for i := range p.Models {
		xAxis = append(xAxis, p.Models[i].Fvalue)
	}

	name = strings.Replace(name, ".pep", "", -1)
	name = strings.Replace(name, ".Pep", "", -1)
	name = strings.Replace(name, ".xml", "", -1)

	for i := 2; i < 8; i++ {

		var obs []float64
		var pos []float64
		var neg []float64

		if i == 2 {

			path := fmt.Sprintf("%s%s%s_2.png", session, string(filepath.Separator), name)
			for j := range p.Models {
				obs = append(obs, p.Models[j].Obs2Distr)
				pos = append(pos, p.Models[j].Model2PosDistr)
				neg = append(neg, p.Models[j].Model2NegDistr)
			}
			printModel("2", path, xAxis, obs, pos, neg)

		} else if i == 3 {

			path := fmt.Sprintf("%s%s%s_3.png", session, string(filepath.Separator), name)
			for j := range p.Models {
				obs = append(obs, p.Models[j].Obs3Distr)
				pos = append(pos, p.Models[j].Model3PosDistr)
				neg = append(neg, p.Models[j].Model3NegDistr)
			}
			printModel("3", path, xAxis, obs, pos, neg)

		} else if i == 4 {

			path := fmt.Sprintf("%s%s%s_4.png", session, string(filepath.Separator), name)
			for j := range p.Models {
				obs = append(obs, p.Models[j].Obs4Distr)
				pos = append(pos, p.Models[j].Model4PosDistr)
				neg = append(neg, p.Models[j].Model4NegDistr)
			}
			printModel("4", path, xAxis, obs, pos, neg)

		} else if i == 5 {

			path := fmt.Sprintf("%s%s%s_5.png", session, string(filepath.Separator), name)
			for j := range p.Models {
				obs = append(obs, p.Models[j].Obs5Distr)
				pos = append(pos, p.Models[j].Model5PosDistr)
				neg = append(neg, p.Models[j].Model5NegDistr)
			}
			printModel("5", path, xAxis, obs, pos, neg)

		} else if i == 6 {

			path := fmt.Sprintf("%s%s%s_6.png", session, string(filepath.Separator), name)
			for j := range p.Models {
				obs = append(obs, p.Models[j].Obs6Distr)
				pos = append(pos, p.Models[j].Model6PosDistr)
				neg = append(neg, p.Models[j].Model6NegDistr)
			}
			printModel("6", path, xAxis, obs, pos, neg)

		} else if i == 7 {

			path := fmt.Sprintf("%s%s%s_7.png", session, string(filepath.Separator), name)
			for j := range p.Models {
				obs = append(obs, p.Models[j].Obs7Distr)
				pos = append(pos, p.Models[j].Model7PosDistr)
				neg = append(neg, p.Models[j].Model7NegDistr)
			}
			printModel("7", path, xAxis, obs, pos, neg)

		}
	}

	return err
}

func printModel(v, path string, xAxis, obs, pos, neg []float64) error {

	p, e := plot.New()
	if e != nil {
		return &err.Error{Type: err.CannotInstantiateStruct, Class: err.FATA, Argument: "plotter"}
	}

	p.Title.Text = "FVAL" + v
	p.X.Label.Text = "FVAL"
	p.Y.Label.Text = "Density"

	obsPts := make(plotter.XYs, len(xAxis))
	posPts := make(plotter.XYs, len(xAxis))
	negPts := make(plotter.XYs, len(xAxis))
	for i := range obs {
		obsPts[i].X = xAxis[i]
		obsPts[i].Y = obs[i]
		posPts[i].X = xAxis[i]
		posPts[i].Y = pos[i]
		negPts[i].X = xAxis[i]
		negPts[i].Y = neg[i]
	}

	e = plotutil.AddLinePoints(p, "Observed", obsPts, "Positive", posPts, "Negative", negPts)
	if e != nil {
		panic(e)
	}

	// Save the plot to a PNG file.
	if err := p.Save(8*vg.Inch, 6*vg.Inch, path); err != nil {
		panic(err)
	}

	// copy to work directory
	sys.CopyFile(path, filepath.Base(path))

	return nil
}

// tdclassifier identifies a PSM as target or Decoy based on the
// presence of the TAG string on <protein> and <alternative_proteins>
func tdclassifier(p PeptideIdentification, tag string) bool {

	// default for TRUE ( DECOY)
	var class = true

	if strings.Contains(string(p.Protein), tag) {
		class = true
	} else {
		class = false
	}

	for i := range p.AlternativeProteins {
		if !strings.Contains(p.AlternativeProteins[i], tag) {
			class = false
		}
		break
	}

	return class
}

// Serialize converts the whle structure to a gob file
func (p *PepXML) Serialize() error {

	var err error

	// create a file
	dataFile, err := os.Create(sys.PepxmlBin())
	if err != nil {
		return err
	}

	dataEncoder := gob.NewEncoder(dataFile)
	goberr := dataEncoder.Encode(p)
	if goberr != nil {
		logrus.Fatal("Cannot save results, Bad format", goberr)
	}
	dataFile.Close()

	return nil
}

// Restore reads philosopher results files and restore the data sctructure
func (p *PepXML) Restore() error {

	file, _ := os.Open(sys.PepxmlBin())

	dec := gob.NewDecoder(file)
	err := dec.Decode(&p)
	if err != nil {
		return errors.New("Could not restore Philosopher result. Please check file path")
	}

	return nil
}

// Serialize converts the whle structure to a gob file
func (p *PepIDList) Serialize(level string) error {

	var err error
	var dest string

	if level == "psm" {
		dest = sys.PsmBin()
	} else if level == "pep" {
		dest = sys.PepBin()
	} else if level == "ion" {
		dest = sys.IonBin()
	} else {
		return errors.New("Cannot seralize Peptide identifications")
	}

	// create a file
	dataFile, err := os.Create(dest)
	if err != nil {
		return err
	}

	dataEncoder := gob.NewEncoder(dataFile)
	goberr := dataEncoder.Encode(p)
	if goberr != nil {
		logrus.Fatal("Cannot save results, Bad format", goberr)
	}
	dataFile.Close()

	return nil
}

// Restore reads philosopher results files and restore the data sctructure
func (p *PepIDList) Restore(level string) error {

	var dest string

	if level == "psm" {
		dest = sys.PsmBin()
	} else if level == "pep" {
		dest = sys.PepBin()
	} else if level == "ion" {
		dest = sys.IonBin()
	} else {
		return errors.New("Cannot seralize Peptide identifications")
	}

	file, _ := os.Open(dest)

	dec := gob.NewDecoder(file)
	err := dec.Decode(&p)
	if err != nil {
		return errors.New("Could not restore Philosopher result. Please check file path")
	}

	return nil
}