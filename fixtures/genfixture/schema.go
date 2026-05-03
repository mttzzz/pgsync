package main

import "fmt"

type fixtureProfile struct {
	Size     string
	Accounts int
	Projects int
	Events   int
}

func profileForSize(size string) (fixtureProfile, error) {
	switch size {
	case "tiny", "":
		return fixtureProfile{Size: "tiny", Accounts: 50, Projects: 150, Events: 500}, nil
	case "medium":
		return fixtureProfile{Size: "medium", Accounts: 2000, Projects: 50000, Events: 200000}, nil
	case "large":
		return fixtureProfile{Size: "large", Accounts: 10000, Projects: 250000, Events: 1000000}, nil
	default:
		return fixtureProfile{}, fmt.Errorf("unsupported fixture size %q", size)
	}
}

func (p fixtureProfile) metadata(seed int64) Metadata {
	return Metadata{
		SchemaVersion:      1,
		Size:               p.Size,
		Seed:               seed,
		ExpectedTableCount: 3,
		ExpectedRows: map[string]int{
			"accounts": p.Accounts,
			"projects": p.Projects,
			"events":   p.Events,
		},
		ExpectedSequences: []string{"accounts_id_seq", "projects_id_seq", "events_id_seq"},
	}
}
