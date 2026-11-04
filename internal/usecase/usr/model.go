package usr

// TokenPair — результат входа и обновления
type TokenPair struct {
	Jwt          string
	RefreshToken string
}

// UpdateProfileReq — изменение своего профиля
type UpdateProfileReq struct {
	Name            *string
	Username        *string
	Password        *string
	CurrentPassword *string
}
