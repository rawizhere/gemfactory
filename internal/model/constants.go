package model

type Gender string

const (
	GenderFemale Gender = "female"
	GenderMale   Gender = "male"
	GenderMixed  Gender = "mixed"
)

func (g Gender) String() string {
	return string(g)
}

func FromBool(isFemale bool) Gender {
	if isFemale {
		return GenderFemale
	}
	return GenderMale
}
