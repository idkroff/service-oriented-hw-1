package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderStatus_CanTransitionTo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		from   OrderStatus
		to     OrderStatus
		expect bool
	}{
		{StatusCreated, StatusPaymentPending, true},
		{StatusCreated, StatusCanceled, true},
		{StatusCreated, StatusPaid, false},
		{StatusPaymentPending, StatusPaid, true},
		{StatusPaymentPending, StatusCanceled, true},
		{StatusPaymentPending, StatusShipped, false},
		{StatusPaid, StatusShipped, true},
		{StatusPaid, StatusCanceled, false},
		{StatusShipped, StatusCompleted, true},
		{StatusCompleted, StatusCanceled, false},
		{StatusCanceled, StatusCreated, false},
	}

	for _, c := range cases {
		c := c
		t.Run(string(c.from)+"_to_"+string(c.to), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.expect, c.from.CanTransitionTo(c.to))
		})
	}
}
