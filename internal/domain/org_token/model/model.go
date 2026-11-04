package model

import "time"

// Main — токен CI организации (ADR-009): только заливка карт и релизов; сам токен не хранится
type Main struct {
	Id            int64
	CreatedAt     time.Time
	OrgId         int64
	CreatedBy     *int64
	CreatedByName string // имя или логин создателя; пусто, если человека удалили
	Name          string
	Hint          string // последние 4 символа токена
	LastUsedAt    *time.Time
}

// Created — токен сразу после создания: значение показывается один раз
type Created struct {
	Main
	Token string
}

type ListReq struct {
	OrgId int64
}
