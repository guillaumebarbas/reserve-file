package service

type Service struct {
	accounts AccountRepository
	limits   LimitProvider
}

func New(accounts AccountRepository, limits LimitProvider) *Service {
	return &Service{accounts: accounts, limits: limits}
}
