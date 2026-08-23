-- +goose Up
-- B2 renames external identity and federation physical tables in place. Apply
-- it before starting application code that uses the lower snake_case names;
-- no legacy-name process may remain live afterward.
ALTER TABLE "sysExternalLoginProvider" RENAME TO sys_external_login_provider;
ALTER TABLE "sysExternalManagedProviderCommand" RENAME TO sys_external_managed_provider_command;
ALTER TABLE "sysExternalOAuthLoginState" RENAME TO sys_external_oauth_login_state;
ALTER TABLE "sysExternalOAuthToken" RENAME TO sys_external_oauth_token;
ALTER TABLE "sysExternalProviderMethod" RENAME TO sys_external_provider_method;
ALTER TABLE "sysExternalUserIdentity" RENAME TO sys_external_user_identity;
ALTER TABLE "sysFederatedNode" RENAME TO sys_federated_node;
ALTER TABLE "sysFederatedNodeConnectionCommand" RENAME TO sys_federated_node_connection_command;

-- +goose Down
-- B2 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
-- +goose StatementBegin
DO $b2$
BEGIN
    RAISE EXCEPTION 'B2 external identity and federation table rename is forward-only';
END
$b2$;
-- +goose StatementEnd
