package facade

import (
	"context"
	"time"
)

// NotificationClient is the semantic, cross-module entry point for creating
// logical notifications. It intentionally has no provider, channel or secret
// configuration methods.
type NotificationClient interface {
	Publish(ctx context.Context, request PublishRequest) (*PublishReceipt, error)
}

// PublishRequest describes the durable business fact that should become an
// in-app notification and/or a third-party enterprise-member delivery. The
// caller supplies stable business idempotency; it never supplies credentials,
// provider URLs, raw provider bodies, or a provider implementation.
type PublishRequest struct {
	EventKey       string `json:"eventKey"`
	IdempotencyKey string `json:"idempotencyKey"`
	// SceneCode selects the published versioned scene independently of EventKey.
	// When omitted, the established semantic Client contract remains available.
	// When supplied, the versioned scene is strict and never falls back to
	// caller content or an older scene configuration.
	SceneCode string `json:"sceneCode"`
	// TemplateVariables are validated only by a published scene template. They
	// are never logged, persisted verbatim, or sent as a provider payload.
	TemplateVariables map[string]any `json:"templateVariables,omitempty"`
	// SendToConfiguredConnection asks for the scene's one saved fixed
	// connection. It does not accept a URL, credential, connection code or
	// target override from the caller.
	SendToConfiguredConnection bool     `json:"sendToConfiguredConnection,omitempty"`
	Audience                   Audience `json:"audience"`
	// ExternalRecipients are optional dynamically supplied third-party members.
	// They never create a SevenFramework inbox recipient.
	ExternalRecipients []ExternalRecipient `json:"externalRecipients,omitempty"`
	// StaticRoutes selects administrator-configured fixed outbound connections.
	// It never carries a URL, credential, group identifier, or raw request body.
	StaticRoutes []StaticRoute `json:"staticRoutes,omitempty"`
	Category     string        `json:"category"`
	Priority     string        `json:"priority"`
	Mandatory    bool          `json:"mandatory"`
	Title        string        `json:"title"`
	Content      string        `json:"content"`
	DeepLink     string        `json:"deepLink,omitempty"`
	ScheduleAt   *time.Time    `json:"scheduleAt,omitempty"`
	ExpiresAt    *time.Time    `json:"expiresAt,omitempty"`
	TraceID      string        `json:"traceId,omitempty"`
	CreatorID    int64         `json:"creatorId,omitempty"`
}

// Audience is a union of explicit users and roles. It is the only source of
// SevenFramework inbox recipients. It may be empty when ExternalRecipients
// contains at least one valid external enterprise member.
type Audience struct {
	UserIDs []int64 `json:"userIds,omitempty"`
	RoleIDs []int64 `json:"roleIds,omitempty"`
}

// ExternalIdentityKind identifies the provider-specific subject format of a
// dynamically supplied enterprise target. It deliberately is not a platform
// user identifier.
type ExternalIdentityKind string

const (
	// ExternalIdentityFeishuOpenID is a Feishu application message recipient.
	ExternalIdentityFeishuOpenID ExternalIdentityKind = "FEISHU_OPEN_ID"
	// ExternalIdentityFeishuChatID is a Feishu application message group.
	ExternalIdentityFeishuChatID ExternalIdentityKind = "FEISHU_CHAT_ID"
	// ExternalIdentityWeComUserID is a WeCom application message recipient.
	ExternalIdentityWeComUserID ExternalIdentityKind = "WECOM_USERID"
)

// ExternalRecipient identifies one third-party enterprise target for this
// business event. ConnectionRef selects the operator-configured application
// scope, while Subject is supplied by the trusted caller for this event.
type ExternalRecipient struct {
	ConnectionRef  string               `json:"connectionRef"`
	IdentityKind   ExternalIdentityKind `json:"identityKind"`
	Subject        string               `json:"subject"`
	ProviderParams map[string]any       `json:"providerParams,omitempty"`
}

// StaticRoute selects a preconfigured static outbound connection for one
// semantic notification. It is deliberately not a provider target and does
// not contain any delivery capability other than the operator-approved
// connection reference.
type StaticRoute struct {
	ConnectionRef string `json:"connectionRef"`
}

// ProviderParameterWarning describes a best-effort optional parameter that
// was ignored. It intentionally carries no raw target, value, credential, or
// provider response body.
type ProviderParameterWarning struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
	Reason   string `json:"reason"`
}

// PublishReceipt is stable for a repeated request with the same idempotency
// key and canonical content.
type PublishReceipt struct {
	NotificationID        string                     `json:"notificationId"`
	Status                string                     `json:"status"`
	MaterializationStatus string                     `json:"materializationStatus"`
	Duplicate             bool                       `json:"duplicate"`
	Warnings              []ProviderParameterWarning `json:"warnings,omitempty"`
}

// InboxQuery is a current-user query. PageCursor is opaque to clients and is
// bound to the authenticated mailbox and selected archive view.
type InboxQuery struct {
	Archived   bool   `query:"archived" json:"archived"`
	PageCursor string `query:"pageCursor" json:"pageCursor,omitempty"`
	PageSize   int    `query:"pageSize" json:"pageSize"`
}

// InboxPage returns compact recipient cards plus a mailbox change token. The
// token is for later synchronization; it is not a page cursor or an ID.
type InboxPage struct {
	Records        []InboxListItem `json:"records"`
	NextPageCursor string          `json:"nextPageCursor,omitempty"`
	ChangeToken    string          `json:"changeToken"`
}

// InboxListItem is the compact card shown only after the user opens the
// message center. It intentionally excludes full content and deep links.
// IDs and versions are strings so browser clients do not lose precision when
// decoding 64-bit values.
type InboxListItem struct {
	RecipientID    string     `json:"recipientId"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary,omitempty"`
	FirstSeenAt    *time.Time `json:"firstSeenAt,omitempty"`
	ReadAt         *time.Time `json:"readAt,omitempty"`
	ArchivedAt     *time.Time `json:"archivedAt,omitempty"`
	MailboxVersion string     `json:"mailboxVersion"`
	CreateTime     time.Time  `json:"createTime"`
	UpdateTime     time.Time  `json:"updateTime"`
}

// InboxDetail is the only inbox response shape that may include the full
// sanitized body and a safe internal deep link. It is loaded only after a user
// explicitly opens one message card.
type InboxDetail struct {
	RecipientID    string     `json:"recipientId"`
	Title          string     `json:"title"`
	Content        string     `json:"content"`
	DeepLink       string     `json:"deepLink,omitempty"`
	FirstSeenAt    *time.Time `json:"firstSeenAt,omitempty"`
	ReadAt         *time.Time `json:"readAt,omitempty"`
	ArchivedAt     *time.Time `json:"archivedAt,omitempty"`
	MailboxVersion string     `json:"mailboxVersion"`
	CreateTime     time.Time  `json:"createTime"`
	UpdateTime     time.Time  `json:"updateTime"`
}

// InboxPreviewItem is a small safe summary used only after the user opens the
// bell Popover. It contains neither full content nor deep-link data.
type InboxPreviewItem struct {
	RecipientID    string    `json:"recipientId"`
	Title          string    `json:"title"`
	Summary        string    `json:"summary,omitempty"`
	MailboxVersion string    `json:"mailboxVersion"`
	CreateTime     time.Time `json:"createTime"`
}

// InboxPreview is the bounded user-triggered unread-preview response.
type InboxPreview struct {
	Records     []InboxPreviewItem `json:"records"`
	MailboxKey  string             `json:"mailboxKey"`
	ChangeToken string             `json:"changeToken"`
}

// InboxChangeQuery asks for a bounded synchronization delta after an already
// open message center has acknowledged an opaque mailbox change token.
type InboxChangeQuery struct {
	AfterChangeToken string `query:"afterChangeToken" json:"afterChangeToken,omitempty"`
	UntilChangeToken string `query:"untilChangeToken" json:"untilChangeToken,omitempty"`
	Limit            int    `query:"limit" json:"limit"`
}

// InboxChanges returns compact upserts and content-free recipient removal
// markers. A client resynchronizes its first page when ResyncRequired is true
// instead of treating a malformed token as an existence oracle.
type InboxChanges struct {
	Upserts             []InboxListItem `json:"upserts"`
	RemovedRecipientIDs []string        `json:"removedRecipientIds"`
	MailboxKey          string          `json:"mailboxKey,omitempty"`
	UnreadCount         int64           `json:"unreadCount"`
	NextChangeToken     string          `json:"nextChangeToken,omitempty"`
	TargetChangeToken   string          `json:"targetChangeToken,omitempty"`
	HasMore             bool            `json:"hasMore"`
	ResyncRequired      bool            `json:"resyncRequired"`
	ServerTime          time.Time       `json:"serverTime"`
}

// InboxMutationRequest carries an optional optimistic-concurrency precondition.
// When it is supplied, a stale version is rejected instead of overwritten.
type InboxMutationRequest struct {
	ExpectedMailboxVersion string `json:"expectedMailboxVersion,omitempty"`
}

// UnreadCount is the shell's current-user-only count response. MailboxKey and
// ChangeToken are opaque cache/synchronization values, never authorization.
type UnreadCount struct {
	MailboxKey  string `json:"mailboxKey"`
	Count       int64  `json:"unreadCount"`
	ChangeToken string `json:"changeToken"`
}

// InboxRealtimeHint is the content-free SSE payload for the currently
// authenticated mailbox. ChangeToken is opaque and is used only to request a
// later compact delta after the user has opened the message center.
type InboxRealtimeHint struct {
	ChangeToken string `json:"changeToken"`
	NewUnread   bool   `json:"newUnread"`
}
