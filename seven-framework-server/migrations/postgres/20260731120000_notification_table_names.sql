-- +goose Up
-- B3 renames notification physical tables in place. Apply it before starting
-- application code that uses the lower snake_case names; no legacy-name
-- process may remain live afterward.
ALTER TABLE "sysNotification" RENAME TO sys_notification;
ALTER TABLE "sysNotificationChannel" RENAME TO sys_notification_channel;
ALTER TABLE "sysNotificationDelivery" RENAME TO sys_notification_delivery;
ALTER TABLE "sysNotificationDeliveryAttempt" RENAME TO sys_notification_delivery_attempt;
ALTER TABLE "sysNotificationDeliveryDiagnosticAudit" RENAME TO sys_notification_delivery_diagnostic_audit;
ALTER TABLE "sysNotificationDeliveryEphemeralContent" RENAME TO sys_notification_delivery_ephemeral_content;
ALTER TABLE "sysNotificationExternalTarget" RENAME TO sys_notification_external_target;
ALTER TABLE "sysNotificationHTTPDeliverySnapshot" RENAME TO sys_notification_http_delivery_snapshot;
ALTER TABLE "sysNotificationMailbox" RENAME TO sys_notification_mailbox;
ALTER TABLE "sysNotificationMaterializationTask" RENAME TO sys_notification_materialization_task;
ALTER TABLE "sysNotificationRecipient" RENAME TO sys_notification_recipient;
ALTER TABLE "sysNotificationSceneBinding" RENAME TO sys_notification_scene_binding;
ALTER TABLE "sysNotificationSceneDefinition" RENAME TO sys_notification_scene_definition;
ALTER TABLE "sysNotificationSceneRevision" RENAME TO sys_notification_scene_revision;
ALTER TABLE "sysNotificationSceneRevisionAudit" RENAME TO sys_notification_scene_revision_audit;
ALTER TABLE "sysNotificationSceneSnapshot" RENAME TO sys_notification_scene_snapshot;
ALTER TABLE "sysNotificationTemplate" RENAME TO sys_notification_template;
ALTER TABLE "sysNotificationTemplateDefinition" RENAME TO sys_notification_template_definition;
ALTER TABLE "sysNotificationTemplateRevision" RENAME TO sys_notification_template_revision;
ALTER TABLE "sysNotificationTemplateRevisionAudit" RENAME TO sys_notification_template_revision_audit;

-- +goose Down
-- B3 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
-- +goose StatementBegin
DO $b3$
BEGIN
    RAISE EXCEPTION 'B3 notification table rename is forward-only';
END
$b3$;
-- +goose StatementEnd
