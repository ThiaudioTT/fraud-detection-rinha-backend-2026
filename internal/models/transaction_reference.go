package models

type ReferenceVector struct {
	ID       int64
	IsFraud  bool
	Distance float64
}
