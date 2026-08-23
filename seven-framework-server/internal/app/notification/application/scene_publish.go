package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-framework/seven-framework-server/internal/common/errors"
)

// Publish keeps the established semantic Client path while making an explicit
// SceneCode strict: a selected versioned scene never falls back to caller
// content or an older scene configuration.
func (s *Service) Publish(ctx context.Context, request facade.PublishRequest) (*facade.PublishReceipt, error) {
	if strings.TrimSpace(request.SceneCode) == "" {
		return s.publishDirect(ctx, request)
	}
	return s.publishScene(ctx, request)
}

type scenePublishResolution struct {
	definition         *domain.SceneDefinition
	revision           *domain.SceneRevision
	templateDefinition *domain.TemplateDefinition
	templateRevision   *domain.TemplateRevision
	channel            *domain.Channel
	rendered           domain.RenderedTemplateRevision
	contentTier        string
	receiverKind       string
	disabled           bool
	snapshotID         *int64
}

type scenePublishPlan struct {
	effectiveAudience facade.Audience
	externalInputs    []facade.ExternalRecipient
	staticRoutes      []facade.StaticRoute
	resolutions       map[string]*scenePublishResolution
	canonicalSubject  string
	canonicalContent  string
	disabledCount     int
}

// publishScene resolves published scene/template/connection records inside
// the acceptance transaction, renders strictly, persists snapshots, then uses
// the same durable target/outbox machinery as V1. It never re-resolves a
// retry against current configuration once an idempotency record exists.
func (s *Service) publishScene(ctx context.Context, request facade.PublishRequest) (*facade.PublishReceipt, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("notification service is not configured")
	}
	if strings.TrimSpace(s.scopeID) == "" {
		return nil, fmt.Errorf("notification scope is not configured")
	}
	fingerprint, err := scenePublishFingerprint(request, s.scopeID)
	if err != nil {
		return nil, err
	}
	receipt := &facade.PublishReceipt{}
	var warnings []facade.ProviderParameterWarning
	err = s.withinTx(ctx, func(txCtx context.Context) error {
		existing, findErr := s.repo.FindLogicalNotificationByIdempotency(txCtx, s.scopeID, strings.TrimSpace(request.EventKey), strings.TrimSpace(request.IdempotencyKey))
		if findErr != nil {
			return findErr
		}
		if existing != nil {
			return s.applyExistingSceneIdempotency(txCtx, existing, fingerprint, request, receipt)
		}

		plan, resolveErr := s.resolveScenePublishPlan(txCtx, request)
		if resolveErr != nil {
			return resolveErr
		}
		acceptedExternal := len(plan.externalInputs) + len(plan.staticRoutes)
		if len(plan.effectiveAudience.UserIDs)+len(plan.effectiveAudience.RoleIDs) == 0 && acceptedExternal == 0 {
			if plan.disabledCount > 0 {
				return sceneDisabledError()
			}
			return apperrors.Params("通知至少需要一个站内信受众或第三方收件人")
		}

		itemRequest := request
		itemRequest.Audience = plan.effectiveAudience
		itemRequest.Title = plan.canonicalSubject
		itemRequest.Content = plan.canonicalContent
		item, audience, createErr := s.newLogicalNotificationBase(itemRequest, acceptedExternal)
		if createErr != nil {
			return createErr
		}
		item.RequestFingerprint = fingerprint

		created, createErr := s.repo.CreateLogicalNotification(txCtx, item)
		if createErr != nil {
			return createErr
		}
		if !created {
			existing, retryFindErr := s.repo.FindLogicalNotificationByIdempotency(txCtx, item.ScopeID, item.EventKey, item.IdempotencyKey)
			if retryFindErr != nil {
				return retryFindErr
			}
			if existing == nil {
				return fmt.Errorf("notification idempotency record disappeared")
			}
			return s.applyExistingSceneIdempotency(txCtx, existing, fingerprint, request, receipt)
		}

		if snapshotErr := s.insertScenePublishSnapshots(txCtx, item, plan, request.TemplateVariables); snapshotErr != nil {
			return snapshotErr
		}
		deferred := audience.HasDeferredAudience() || len(audience.UserIDs) > inlineRecipientLimit || isScheduled(item.ScheduleAt, s.now())
		if deferred {
			if err := s.createMaterializationTask(txCtx, item, audience); err != nil {
				return err
			}
		} else {
			recipients := s.newRecipients(item, audience.UserIDs)
			createdRecipients, insertErr := s.repo.InsertInboxRecipients(txCtx, recipients)
			if insertErr != nil {
				return insertErr
			}
			if err := s.appendInboxChangedIntents(txCtx, createdRecipients, true); err != nil {
				return err
			}
		}

		preparedExternal, preparedWarnings, prepareErr := s.prepareExternalRecipients(txCtx, plan.externalInputs)
		if prepareErr != nil {
			return prepareErr
		}
		preparedStatic, staticErr := s.prepareStaticRoutes(txCtx, plan.staticRoutes)
		if staticErr != nil {
			return staticErr
		}
		if err := s.createExternalTargetsAndDeliveriesWithScene(txCtx, item, preparedExternal, plan.externalRenders()); err != nil {
			return err
		}
		if err := s.createStaticRouteDeliveriesWithScene(txCtx, item, preparedStatic, plan.staticRender()); err != nil {
			return err
		}

		intent := domain.IntentMessage{NotificationID: item.ID, ScopeID: item.ScopeID}
		if err := s.repo.AppendOutbox(txCtx, &domain.OutboxEvent{
			ID:            s.nextID(),
			EventID:       "notification-intent:" + item.NotificationID,
			ScopeID:       item.ScopeID,
			EventType:     domain.OutboxEventNotificationIntent,
			AggregateType: domain.OutboxAggregateNotification,
			AggregateID:   item.NotificationID,
			Payload:       mustJSON(intent),
			Status:        "PENDING",
		}); err != nil {
			return err
		}
		receipt.NotificationID = item.NotificationID
		receipt.Status = item.Status
		receipt.MaterializationStatus = materializationStatus(deferred, item.Status)
		warnings = preparedWarnings
		receipt.Warnings = append([]facade.ProviderParameterWarning(nil), preparedWarnings...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logProviderParameterWarnings(warnings)
	return receipt, nil
}

func (s *Service) resolveScenePublishPlan(ctx context.Context, request facade.PublishRequest) (*scenePublishPlan, error) {
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	plan := &scenePublishPlan{
		resolutions: make(map[string]*scenePublishResolution),
	}
	needed := make(map[string]struct{})
	if len(request.Audience.UserIDs)+len(request.Audience.RoleIDs) > 0 {
		needed[domain.SceneReceiverKindInApp] = struct{}{}
	}
	for _, recipient := range request.ExternalRecipients {
		kind, kindErr := domain.SceneReceiverKindForExternalIdentity(string(recipient.IdentityKind))
		if kindErr != nil {
			return nil, apperrors.Params(kindErr.Error())
		}
		if _, subjectErr := domain.NormalizeExternalTargetSubject(string(recipient.IdentityKind), recipient.Subject); subjectErr != nil {
			return nil, apperrors.Params("第三方目标标识无效")
		}
		needed[kind] = struct{}{}
	}
	if request.SendToConfiguredConnection {
		needed[domain.SceneReceiverKindFixedConnection] = struct{}{}
	}
	if len(needed) == 0 {
		return nil, apperrors.Params("新版场景通知至少需要一个接收对象")
	}
	kinds := make([]string, 0, len(needed))
	for kind := range needed {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		resolution, resolveErr := s.resolveSceneReceiver(ctx, repo, strings.TrimSpace(request.SceneCode), kind, request.TemplateVariables)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if resolution != nil {
			plan.resolutions[kind] = resolution
			if resolution.disabled {
				plan.disabledCount++
			}
		}
	}

	if resolution := plan.resolutions[domain.SceneReceiverKindInApp]; resolution != nil {
		if !resolution.disabled {
			plan.effectiveAudience = request.Audience
			plan.canonicalSubject, plan.canonicalContent = renderedSceneContent(resolution.rendered)
		}
	} else if len(request.Audience.UserIDs)+len(request.Audience.RoleIDs) > 0 {
		return nil, sceneNotPublishedError()
	}

	for _, recipient := range request.ExternalRecipients {
		kind, _ := domain.SceneReceiverKindForExternalIdentity(string(recipient.IdentityKind))
		resolution := plan.resolutions[kind]
		switch {
		case resolution == nil:
			return nil, sceneNotPublishedError()
		case resolution.disabled:
			// A deliberately disabled published scene never falls back to caller
			// content or a caller-selected connection. It creates no target/outbox.
			continue
		default:
			if strings.TrimSpace(recipient.ConnectionRef) != "" && strings.TrimSpace(recipient.ConnectionRef) != resolution.revision.ConnectionRef {
				return nil, apperrors.Params("第三方收件人连接与已发布场景发送方式不一致")
			}
			copy := recipient
			copy.ConnectionRef = resolution.revision.ConnectionRef
			plan.externalInputs = append(plan.externalInputs, copy)
		}
	}
	if request.SendToConfiguredConnection {
		resolution := plan.resolutions[domain.SceneReceiverKindFixedConnection]
		if resolution == nil {
			return nil, sceneNotPublishedError()
		}
		if !resolution.disabled {
			plan.staticRoutes = []facade.StaticRoute{{ConnectionRef: resolution.revision.ConnectionRef}}
		}
	}

	if strings.TrimSpace(plan.canonicalSubject) == "" || strings.TrimSpace(plan.canonicalContent) == "" {
		for _, kind := range kinds {
			if resolution := plan.resolutions[kind]; resolution != nil && !resolution.disabled {
				plan.canonicalSubject, plan.canonicalContent = renderedSceneContent(resolution.rendered)
				break
			}
		}
	}
	return plan, nil
}

// resolveSceneReceiver locks one configured scene identity so template and
// connection selection cannot move midway through acceptance. A missing or
// draft-only scene is represented by nil so the caller receives the explicit
// SCENE_NOT_PUBLISHED result.
func (s *Service) resolveSceneReceiver(ctx context.Context, repo domain.SceneRevisionRepository, sceneCode, receiverKind string, values map[string]any) (*scenePublishResolution, error) {
	definition, err := repo.LockSceneDefinitionByCodeAndReceiverKind(ctx, s.sceneRevisionScopeID(), sceneCode, receiverKind)
	if err != nil {
		return nil, err
	}
	if definition == nil || definition.CurrentPublishedRevisionID == nil {
		return nil, nil
	}
	revision, err := repo.FindSceneRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
	if err != nil {
		return nil, err
	}
	if revision == nil || revision.State != domain.SceneRevisionStatePublished {
		return nil, domain.ErrSceneRevisionConflict
	}
	templateDefinition, templateRevision, err := s.loadSceneTemplateRevision(ctx, revision.TemplateRevisionID)
	if err != nil {
		return nil, err
	}
	resolution := &scenePublishResolution{
		definition:         definition,
		revision:           revision,
		templateDefinition: templateDefinition,
		templateRevision:   templateRevision,
		receiverKind:       receiverKind,
		disabled:           !revision.Enabled,
	}
	if resolution.disabled {
		return resolution, nil
	}
	if err := s.validateSceneConnectionAtAcceptance(ctx, definition, revision); err != nil {
		return nil, err
	}
	if strings.TrimSpace(revision.ConnectionRef) != "" {
		channel, findErr := s.repo.FindChannelByCode(ctx, revision.ConnectionRef)
		if findErr != nil {
			return nil, findErr
		}
		resolution.channel = channel
	}
	variables, err := domain.TemplateVariablesFromJSON(templateRevision.VariableSchemaJSON)
	if err != nil {
		return nil, apperrors.Operation("已发布模板变量配置无效")
	}
	resolution.contentTier = domain.ContentTierForTemplateVariables(variables)
	rendered, err := domain.RenderTemplateRevision(domain.TemplateRevisionDraft{
		TemplateName:     templateDefinition.TemplateName,
		Locale:           templateDefinition.Locale,
		SubjectTemplate:  templateRevision.SubjectTemplate,
		TextTemplate:     templateRevision.TextTemplate,
		HTMLTemplate:     templateRevision.HTMLTemplate,
		MarkdownTemplate: templateRevision.MarkdownTemplate,
		Variables:        variables,
	}, values)
	if err != nil {
		return nil, apperrors.Params("场景模板变量校验或渲染失败")
	}
	resolution.rendered = rendered
	return resolution, nil
}

func (s *Service) loadSceneTemplateRevision(ctx context.Context, revisionID int64) (*domain.TemplateDefinition, *domain.TemplateRevision, error) {
	templateRepo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, nil, err
	}
	revision, err := templateRepo.FindTemplateRevisionByID(ctx, revisionID)
	if err != nil {
		return nil, nil, err
	}
	if revision == nil || (revision.State != domain.TemplateRevisionStatePublished && revision.State != domain.TemplateRevisionStateSuperseded) {
		return nil, nil, apperrors.Operation("场景引用的模板版本不可用")
	}
	definition, err := templateRepo.FindTemplateDefinitionByID(ctx, revision.TemplateDefinitionID)
	if err != nil {
		return nil, nil, err
	}
	if !s.templateDefinitionBelongsToCurrentScope(definition) {
		return nil, nil, apperrors.Operation("场景引用的模板不属于当前作用域")
	}
	return definition, revision, nil
}

func (s *Service) validateSceneConnectionAtAcceptance(ctx context.Context, definition *domain.SceneDefinition, revision *domain.SceneRevision) error {
	if definition == nil || revision == nil {
		return domain.ErrSceneRevisionNotFound
	}
	if definition.ReceiverKind == domain.SceneReceiverKindInApp {
		return domain.ValidateSceneConnection(definition.ReceiverKind, nil, revision.ConnectionRef)
	}
	channel, err := s.repo.FindChannelByCode(ctx, revision.ConnectionRef)
	if err != nil {
		return err
	}
	if channel == nil || !s.channelBelongsToCurrentScope(channel) || channel.Status != domain.ChannelStatusEnabled {
		return apperrors.Operation("场景发送方式不可用")
	}
	if err := domain.ValidateSceneConnection(definition.ReceiverKind, channel, revision.ConnectionRef); err != nil {
		return apperrors.Operation("场景发送方式与接收对象不匹配")
	}
	if err := s.validateSceneChannelReadiness(*channel, true); err != nil {
		return err
	}
	if domain.SceneConnectionDigest(channel) != revision.ConnectionDigest {
		return apperrors.Operation("场景发送方式已变更，请新建场景版本")
	}
	return nil
}

func (s *Service) insertScenePublishSnapshots(ctx context.Context, notification *domain.LogicalNotification, plan *scenePublishPlan, values map[string]any) error {
	if notification == nil || plan == nil {
		return fmt.Errorf("notification scene acceptance state is invalid")
	}
	variableDigest, err := sceneVariablesDigest(values)
	if err != nil {
		return apperrors.Params("场景模板变量格式无效")
	}
	kinds := make([]string, 0, len(plan.resolutions))
	for kind := range plan.resolutions {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return err
	}
	for _, kind := range kinds {
		resolution := plan.resolutions[kind]
		if resolution == nil || resolution.definition == nil || resolution.revision == nil || resolution.templateDefinition == nil || resolution.templateRevision == nil {
			continue
		}
		resolutionValue := domain.SceneSnapshotResolutionAccepted
		renderedDigest := sceneRenderedDigest(resolution.rendered)
		if resolution.disabled {
			resolutionValue = domain.SceneSnapshotResolutionDisabled
			renderedDigest = digest("scene-disabled", resolution.templateRevision.ContentDigest)
		}
		snapshotID := s.nextID()
		snapshot := &domain.SceneSnapshot{
			ID:                    snapshotID,
			NotificationID:        notification.ID,
			ScopeID:               notification.ScopeID,
			SceneCode:             resolution.definition.SceneCode,
			ReceiverKind:          resolution.receiverKind,
			SceneDefinitionID:     resolution.definition.ID,
			SceneRevisionID:       resolution.revision.ID,
			TemplateDefinitionID:  resolution.templateDefinition.ID,
			TemplateRevisionID:    resolution.templateRevision.ID,
			ConnectionRef:         resolution.revision.ConnectionRef,
			ConnectionDigest:      resolution.revision.ConnectionDigest,
			TemplateContentDigest: resolution.templateRevision.ContentDigest,
			RenderedDigest:        renderedDigest,
			VariableDigest:        variableDigest,
			Resolution:            resolutionValue,
		}
		if err := repo.InsertSceneSnapshot(ctx, snapshot); err != nil {
			return err
		}
		resolution.snapshotID = int64Ptr(snapshotID)
	}
	return nil
}

func (p *scenePublishPlan) externalRenders() map[string]sceneDeliveryRender {
	if p == nil {
		return nil
	}
	result := make(map[string]sceneDeliveryRender)
	for _, resolution := range p.resolutions {
		if resolution == nil || resolution.disabled || resolution.revision == nil || resolution.templateDefinition == nil || resolution.receiverKind == domain.SceneReceiverKindInApp || resolution.receiverKind == domain.SceneReceiverKindFixedConnection {
			continue
		}
		result[strings.ToUpper(resolution.receiverKind)+"\x00"+strings.TrimSpace(resolution.revision.ConnectionRef)] = sceneDeliveryRender{
			SceneCode:        resolution.definition.SceneCode,
			TemplateCode:     resolution.templateDefinition.TemplateCode,
			RenderedSubject:  normalizedRenderedSubject(resolution.rendered.Subject),
			RenderedText:     resolution.rendered.Text,
			RenderedHTML:     resolution.rendered.HTML,
			RenderedMarkdown: resolution.rendered.Markdown,
			ContentTier:      resolution.contentTier,
			SceneSnapshotID:  resolution.snapshotID,
		}
	}
	return result
}

func (p *scenePublishPlan) staticRender() *sceneDeliveryRender {
	if p == nil {
		return nil
	}
	resolution := p.resolutions[domain.SceneReceiverKindFixedConnection]
	if resolution == nil {
		return nil
	}
	if resolution.disabled || resolution.definition == nil || resolution.templateDefinition == nil {
		return nil
	}
	return &sceneDeliveryRender{
		SceneCode:        resolution.definition.SceneCode,
		TemplateCode:     resolution.templateDefinition.TemplateCode,
		RenderedSubject:  normalizedRenderedSubject(resolution.rendered.Subject),
		RenderedText:     resolution.rendered.Text,
		RenderedHTML:     resolution.rendered.HTML,
		RenderedMarkdown: resolution.rendered.Markdown,
		ContentTier:      resolution.contentTier,
		SceneSnapshotID:  resolution.snapshotID,
	}
}

func sceneExternalInputKey(identityKind, connectionRef string) string {
	return strings.ToUpper(strings.TrimSpace(identityKind)) + "\x00" + strings.TrimSpace(connectionRef)
}

func renderedSceneContent(rendered domain.RenderedTemplateRevision) (string, string) {
	return normalizedRenderedSubject(rendered.Subject), choose(strings.TrimSpace(rendered.Text), strings.TrimSpace(rendered.Markdown), strings.TrimSpace(rendered.HTML))
}

func normalizedRenderedSubject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "通知"
	}
	return value
}

func (s *Service) applyExistingSceneIdempotency(ctx context.Context, existing *domain.LogicalNotification, fingerprint string, request facade.PublishRequest, receipt *facade.PublishReceipt) error {
	if existing == nil || receipt == nil || existing.RequestFingerprint != fingerprint {
		return idempotencyConflict()
	}
	snapshotsRepo, err := s.sceneRevisionRepository()
	if err != nil {
		return err
	}
	snapshots, err := snapshotsRepo.ListSceneSnapshotsByNotificationID(ctx, existing.ID)
	if err != nil {
		return err
	}
	byKind := make(map[string]domain.SceneSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byKind[snapshot.ReceiverKind] = snapshot
	}
	for _, recipient := range request.ExternalRecipients {
		kind, kindErr := domain.SceneReceiverKindForExternalIdentity(string(recipient.IdentityKind))
		if kindErr != nil {
			return apperrors.Params(kindErr.Error())
		}
		if strings.TrimSpace(recipient.ConnectionRef) == "" {
			continue
		}
		if snapshot, found := byKind[kind]; found && strings.TrimSpace(snapshot.ConnectionRef) != "" && strings.TrimSpace(recipient.ConnectionRef) != strings.TrimSpace(snapshot.ConnectionRef) {
			return idempotencyConflict()
		}
	}
	incoming := &domain.LogicalNotification{RequestFingerprint: fingerprint}
	return applyIdempotentReceipt(existing, incoming, receipt)
}

func sceneNotPublishedError() error {
	return apperrors.ObjectState("场景尚未发布").WithDetails(map[string]string{"reasonCode": "SCENE_NOT_PUBLISHED"})
}

func sceneDisabledError() error {
	return apperrors.ObjectState("场景已停用").WithDetails(map[string]string{"reasonCode": "SCENE_DISABLED"})
}

func scenePublishFingerprint(request facade.PublishRequest, scopeID string) (string, error) {
	audience, err := domain.NormalizeOptionalAudience(request.Audience.UserIDs, request.Audience.RoleIDs)
	if err != nil {
		return "", apperrors.Params(err.Error())
	}
	type target struct {
		IdentityKind       string `json:"identityKind"`
		SubjectDigest      string `json:"subjectDigest"`
		ProviderParamsJSON string `json:"providerParamsJson"`
	}
	targets := make([]target, 0, len(request.ExternalRecipients))
	for _, recipient := range request.ExternalRecipients {
		identityKind := strings.ToUpper(strings.TrimSpace(string(recipient.IdentityKind)))
		subject, normalizeErr := domain.NormalizeExternalTargetSubject(identityKind, recipient.Subject)
		if normalizeErr != nil {
			return "", apperrors.Params("第三方目标标识无效")
		}
		params, encodeErr := json.Marshal(recipient.ProviderParams)
		if encodeErr != nil {
			return "", apperrors.Params("第三方可选参数格式无效")
		}
		subjectSum := sha256.Sum256([]byte(identityKind + "\x00" + subject))
		targets = append(targets, target{IdentityKind: identityKind, SubjectDigest: hex.EncodeToString(subjectSum[:]), ProviderParamsJSON: string(params)})
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].IdentityKind != targets[j].IdentityKind {
			return targets[i].IdentityKind < targets[j].IdentityKind
		}
		if targets[i].SubjectDigest != targets[j].SubjectDigest {
			return targets[i].SubjectDigest < targets[j].SubjectDigest
		}
		return targets[i].ProviderParamsJSON < targets[j].ProviderParamsJSON
	})
	variableDigest, err := sceneVariablesDigest(request.TemplateVariables)
	if err != nil {
		return "", apperrors.Params("场景模板变量格式无效")
	}
	canonical := struct {
		ScopeID                    string                  `json:"scopeId"`
		EventKey                   string                  `json:"eventKey"`
		IdempotencyKey             string                  `json:"idempotencyKey"`
		SceneCode                  string                  `json:"sceneCode"`
		Audience                   domain.AudienceSnapshot `json:"audience"`
		Targets                    []target                `json:"targets"`
		SendToConfiguredConnection bool                    `json:"sendToConfiguredConnection"`
		TemplateVariablesDigest    string                  `json:"templateVariablesDigest"`
		Category                   string                  `json:"category"`
		Priority                   string                  `json:"priority"`
		Mandatory                  bool                    `json:"mandatory"`
		DeepLink                   string                  `json:"deepLink"`
		ScheduleAt                 string                  `json:"scheduleAt"`
		ExpiresAt                  string                  `json:"expiresAt"`
	}{
		ScopeID: scopeID, EventKey: strings.TrimSpace(request.EventKey), IdempotencyKey: strings.TrimSpace(request.IdempotencyKey), SceneCode: strings.TrimSpace(request.SceneCode), Audience: audience,
		Targets: targets, SendToConfiguredConnection: request.SendToConfiguredConnection, TemplateVariablesDigest: variableDigest,
		Category: strings.ToUpper(defaultNotificationCategory(request.Category)), Priority: strings.ToUpper(defaultNotificationPriority(request.Priority)), Mandatory: request.Mandatory,
		DeepLink: strings.TrimSpace(request.DeepLink),
	}
	if request.ScheduleAt != nil {
		canonical.ScheduleAt = request.ScheduleAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	if request.ExpiresAt != nil {
		canonical.ExpiresAt = request.ExpiresAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func sceneVariablesDigest(values map[string]any) (string, error) {
	if values == nil {
		values = map[string]any{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func sceneRenderedDigest(rendered domain.RenderedTemplateRevision) string {
	return digest(rendered.Subject, rendered.Text, rendered.HTML, rendered.Markdown)
}
