package molecule

import (
	"sync"

	cmn "github.com/RxnWeaver/rxnweaver/common"
)

// nextMolIdHolder is a synchronised struct used to assign a
// globally-unique ID to each molecule.
type nextMolIdHolder struct {
	mu     sync.Mutex
	nextId uint32
}

// The only instance of `nextMolIdHolder`.
var nextMolId nextMolIdHolder

func nextMoleculeId() uint32 {
	nextMolId.mu.Lock()
	defer nextMolId.mu.Unlock()

	nextMolId.nextId++
	return nextMolId.nextId
}

// molecules holds all the molecules that are currently alive.
type molecules struct {
	allMolecules map[uint32]*Molecule
}

// MoleculeWithId answers the molecule instance with the given ID, if
// one such exists.
func (ms *molecules) MoleculeWithId(id uint32) *Molecule {
	if mol, ok := ms.allMolecules[id]; ok {
		return mol
	}

	return nil
}

// Clear sends a termination request to all the alive molecules, and
// stops tracking them.
func (ms *molecules) Clear() {
	for id, mol := range ms.allMolecules {
		msg := InMessage{ReqExit, 0, nil, nil}
		mol.InChannel() <- msg
		delete(ms.allMolecules, id)
	}
}

// The only instance of `molecules`.
var AllMolecules molecules

// Initialise the global molecule cache.
func init() {
	AllMolecules.allMolecules = make(map[uint32]*Molecule)
}

// Molecule represents a chemical molecule.
//
// It holds information concerning its atom, bonds, rings, etc.  Note
// that a molecule is expected to be a single connected component.
type Molecule struct {
	id uint32 // The globally-unique ID of this molecule.

	// Channel on which this molecule receives requests and
	// notifications.
	inChannel chan InMessage

	atoms       []*_Atom       // List of atoms in this molecule.
	bonds       []*_Bond       // List of bonds in this molecule.
	rings       []*_Ring       // List of rings in this molecule.
	ringSystems []*_RingSystem // List of ring systems in this molecule.

	nextAtomIid      uint16 // Running number for atom input IDs.
	nextBondId       uint16 // Running number for bond IDs.
	nextRingId       uint8  // Running number for ring IDs.
	nextRingSystemId uint8  // Running number for ring system IDs.

	vendor           string // Optional string identifying the supplier.
	vendorMoleculeId string // Optional supplier-specified ID.

	attributes []Attribute // Optional list of annotations.

	dists [][]int // Matrix of pair-wise distances between atoms.
	paths [][]int // Lists of pair-wise paths between atoms.
}

// New creates and initialises a molecule.
func New() *Molecule {
	mol := new(Molecule)
	mol.id = nextMoleculeId()

	mol.inChannel = make(chan InMessage, ReqChanSize)

	mol.atoms = make([]*_Atom, 0, cmn.ListSizeLarge)
	mol.bonds = make([]*_Bond, 0, cmn.ListSizeLarge)
	mol.rings = make([]*_Ring, 0, cmn.ListSizeSmall)
	mol.ringSystems = make([]*_RingSystem, 0, cmn.ListSizeSmall)

	mol.attributes = make([]Attribute, 0, cmn.ListSizeTiny)

	// Start the molecule's event loop.
	go mol.run()

	return mol
}

// NewAtomBuilder answers a new atom builder.
func (m *Molecule) NewAtomBuilder() *AtomBuilder {
	return &AtomBuilder{m, nil}
}

// NewBondBuilder answers a new bond builder.
func (m *Molecule) NewBondBuilder() *BondBuilder {
	return &BondBuilder{m, nil}
}

// Id answers the globally-unique ID of this molecule.
func (m *Molecule) Id() uint32 {
	return m.id
}

// InChannel answers the input channel of this molecule.
func (m *Molecule) InChannel() chan InMessage {
	return m.inChannel
}

// run is the event loop of this molecule.
//
// It serves as the entry point of all in-coming requests from all
// external agents.  For each request, an appropriate processing is
// then performed, and the result returned on the channel that is part
// of that request.
func (m *Molecule) run() {
	// Register this molecule in the cache.
	AllMolecules.allMolecules[m.id] = m

	// Unregister this molecule from the cache when done.
	defer delete(AllMolecules.allMolecules, m.id)

	alive := true

liveloop:
	for {
		if !alive {
			break liveloop
		}

		select {
		case msg := <-m.inChannel:

			switch msg.Request {
			case ReqExit:
				alive = false

			default:
				m.processInMessage(msg)
			}
		}
	}
}

// processInMessage is the workhorse function of this molecule.
func (m *Molecule) processInMessage(msg InMessage) {
	// TODO(js): Implement.
}

// atomWithIid answers the atom for the given input ID, if found.
// Answers `nil` otherwise.
func (m *Molecule) atomWithIid(id uint16) *_Atom {
	for _, a := range m.atoms {
		if a.iId == id {
			return a
		}
	}

	return nil
}

// atomWithNid answers the atom for the given normalised ID, if found.
// Answers `nil` otherwise.
func (m *Molecule) atomWithNid(id uint16) *_Atom {
	for _, a := range m.atoms {
		if a.nId == id {
			return a
		}
	}

	return nil
}

// bondWithId answers the bond for the given ID, if found.  Answers
// `nil` otherwise.
func (m *Molecule) bondWithId(id uint16) *_Bond {
	for _, b := range m.bonds {
		if b.id == id {
			return b
		}
	}

	return nil
}

// ringWithId answers the ring for the given ID, if found.  Answers
// `nil` otherwise.
func (m *Molecule) ringWithId(id uint8) *_Ring {
	for _, r := range m.rings {
		if r.id == id {
			return r
		}
	}

	return nil
}

// bondBetween answers the bond between the two given atoms, if one
// such exists.  Answers `nil` otherwise.
//
// Note that the two given atoms are represented by their input IDs,
// NOT normalised IDs.
func (m *Molecule) bondBetween(a1id, a2id uint16) *_Bond {
	for _, b := range m.bonds {
		if (b.a1 == a1id && b.a2 == a2id) || (b.a2 == a1id && b.a1 == a2id) {
			return b
		}
	}

	return nil
}

// bondCount answers the total number of bonds of the given type in
// this molecule.
func (m *Molecule) bondCount(typ cmn.BondType) int {
	c := 0
	for _, b := range m.bonds {
		if b.bType == typ {
			c++
		}
	}

	return c
}

// singleBondCount answers the total number of single bonds in this
// molecule.
func (m *Molecule) singleBondCount() int {
	return m.bondCount(cmn.BondTypeSingle)
}

// doubleBondCount answers the total number of double bonds in this
// molecule.
func (m *Molecule) doubleBondCount() int {
	return m.bondCount(cmn.BondTypeDouble)
}

// tripleBondCount answers the total number of triple bonds in this
// molecule.
func (m *Molecule) tripleBondCount() int {
	return m.bondCount(cmn.BondTypeTriple)
}

// aromaticRingCount answers the number of aromatic rings in this
// molecule.
func (m *Molecule) aromaticRingCount() int {
	c := 0
	for _, r := range m.rings {
		if r.isAro {
			c++
		}
	}

	return c
}

// aromaticRingSystemCount answers the number of aromatic ring systems
// in this molecule.
func (m *Molecule) aromaticRingSystemCount() int {
	c := 0
	for _, rs := range m.ringSystems {
		if rs.isAro {
			c++
		}
	}

	return c
}
