package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type User struct {
	ID primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name string `json:"name"`
	LastName string `json:"lastName"`
	Email string `json:"email"`
	Password string `json:"password"`
	Role string `json:"role"`
	// ... other fields like email, name, etc.
}
type LoginRequest struct {
    Name string `json:"name"`
    Password string `json:"password"`
}


// TODO: Role enum will be added . this should be by default [superadmin, admin] and should be taken from db . by the reqiurement of the project it can be extended
