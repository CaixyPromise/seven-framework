-- +goose Up
-- B3 renames notification physical tables in place. Apply it before starting
-- application code that uses the lower snake_case names; no legacy-name
-- process may remain live afterward.
RENAME TABLE
    sysNotification TO sys_notification,
    sysNotificationChannel TO sys_notification_channel,
    sysNotificationDelivery TO sys_notification_delivery,
    sysNotificationDeliveryAttempt TO sys_notification_delivery_attempt,
    sysNotificationDeliveryDiagnosticAudit TO sys_notification_delivery_diagnostic_audit,
    sysNotificationDeliveryEphemeralContent TO sys_notification_delivery_ephemeral_content,
    sysNotificationExternalTarget TO sys_notification_external_target,
    sysNotificationHTTPDeliverySnapshot TO sys_notification_http_delivery_snapshot,
    sysNotificationMailbox TO sys_notification_mailbox,
    sysNotificationMaterializationTask TO sys_notification_materialization_task,
    sysNotificationRecipient TO sys_notification_recipient,
    sysNotificationSceneBinding TO sys_notification_scene_binding,
    sysNotificationSceneDefinition TO sys_notification_scene_definition,
    sysNotificationSceneRevision TO sys_notification_scene_revision,
    sysNotificationSceneRevisionAudit TO sys_notification_scene_revision_audit,
    sysNotificationSceneSnapshot TO sys_notification_scene_snapshot,
    sysNotificationTemplate TO sys_notification_template,
    sysNotificationTemplateDefinition TO sys_notification_template_definition,
    sysNotificationTemplateRevision TO sys_notification_template_revision,
    sysNotificationTemplateRevisionAudit TO sys_notification_template_revision_audit;

-- +goose Down
-- B3 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'B3 notification table rename is forward-only';
