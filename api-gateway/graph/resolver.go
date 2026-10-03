package graph

import userpb "github.com/chetansachan/socialflow/proto"

type Resolver struct {
	UserClient userpb.UserServiceClient
}