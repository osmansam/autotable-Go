package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type User struct {
	ID primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
	Role string `json:"role"`
	// ... other fields like email, name, etc.
}
type LoginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}
type TokenRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// TODO: Role enum will be added . this should be by default [superadmin, admin] and should be taken from db . by the reqiurement of the project it can be extended