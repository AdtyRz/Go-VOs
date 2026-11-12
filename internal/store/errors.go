package store

import (
	"errors"
)

var (
	ErrNotFound         = errors.New("data tidak ditemukan")
	ErrConflict         = errors.New("data bertentangan")
	ErrInvalid          = errors.New("data tidak valid")
	ErrAlreadyUsed      = errors.New("token sudah digunakan")
	ErrTokenInvalid     = errors.New("token tidak valid atau sudah digunakan")
	ErrTokenRevoked     = errors.New("token sudah dicabut")
	ErrTokenExpired     = errors.New("token sudah kedaluwarsa")
	ErrElectionClosed   = errors.New("pemilihan tidak sedang berlangsung")
	ErrSessionInvalid   = errors.New("sesi memilih tidak valid atau kedaluwarsa")
	ErrCandidateInvalid = errors.New("pasangan calon tidak tersedia")
	ErrVoteNotAllowed   = errors.New("suara tidak dapat diterima")
	ErrNoActiveElection = errors.New("belum ada pemilihan yang berlangsung")
	ErrLocked           = errors.New("akun sementara dikunci")
	ErrInactive         = errors.New("akun tidak aktif")
	ErrVotesExist       = errors.New("pasangan calon sudah memiliki suara")
	ErrElectionLocked   = errors.New("pemilihan sudah berjalan dan data tidak dapat diubah")
)
