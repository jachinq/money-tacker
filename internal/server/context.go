package server

import (
	"context"
	"money-tacker/internal/store"
)

type ctxKey int

const userKey ctxKey = 1

func withUser(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func userFrom(ctx context.Context) *store.User {
	u, _ := ctx.Value(userKey).(*store.User)
	return u
}
