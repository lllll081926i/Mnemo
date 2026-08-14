package store

import (
	"os"
	"sort"

	"mnemo-go/internal/model"
)

const accountsFile = "accounts.json"

// ListAccounts returns all persisted accounts sorted by order then user id.
func (s *Store) ListAccounts() ([]*model.Account, error) {
	var list []*model.Account
	err := s.readJSON(accountsFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Order != list[j].Order {
			return list[i].Order < list[j].Order
		}
		return list[i].UserID < list[j].UserID
	})
	return list, nil
}

// GetAccount returns one account by user id.
func (s *Store) GetAccount(userID string) (*model.Account, error) {
	list, err := s.ListAccounts()
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		if a.UserID == userID {
			return a, nil
		}
	}
	return nil, os.ErrNotExist
}

// SaveAccount upserts an account.
func (s *Store) SaveAccount(account *model.Account) error {
	if account == nil || account.UserID == "" {
		return errInvalid("account user_id is empty")
	}
	list, err := s.ListAccounts()
	if err != nil {
		return err
	}
	replaced := false
	for i, a := range list {
		if a.UserID == account.UserID {
			list[i] = account
			replaced = true
			break
		}
	}
	if !replaced {
		if account.Order == 0 {
			account.Order = int64(len(list)) + 1
		}
		list = append(list, account)
	}
	return s.writeJSON(accountsFile, list)
}

// DeleteAccount removes an account by user id.
func (s *Store) DeleteAccount(userID string) error {
	list, err := s.ListAccounts()
	if err != nil {
		return err
	}
	out := list[:0]
	for _, a := range list {
		if a.UserID != userID {
			out = append(out, a)
		}
	}
	return s.writeJSON(accountsFile, out)
}

// UpdateAccountToken refreshes the token of an account.
func (s *Store) UpdateAccountToken(userID string, token *model.TokenInfo) error {
	a, err := s.GetAccount(userID)
	if err != nil {
		return err
	}
	a.Token = token
	return s.SaveAccount(a)
}