package domain

import "context"

type Repository interface {
	Create(context.Context, Reservation) error
	Get(context.Context, string) (Reservation, error)
	Update(context.Context, Reservation) error
	List(context.Context) ([]Reservation, error)
}

type Releaser interface {
	Release(counterKey string, cost int64)
}
