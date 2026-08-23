-- +goose Up
-- B2 renames external identity and federation physical tables in place. Apply
-- it before starting application code that uses the lower snake_case names;
-- no legacy-name process may remain live afterward.
RENAME TABLE
    sysExternalLoginProvider TO sys_external_login_provider,
    sysExternalManagedProviderCommand TO sys_external_managed_provider_command,
    sysExternalOAuthLoginState TO sys_external_oauth_login_state,
    sysExternalOAuthToken TO sys_external_oauth_token,
    sysExternalProviderMethod TO sys_external_provider_method,
    sysExternalUserIdentity TO sys_external_user_identity,
    sysFederatedNode TO sys_federated_node,
    sysFederatedNodeConnectionCommand TO sys_federated_node_connection_command;

-- +goose Down
-- B2 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'B2 external identity and federation table rename is forward-only';
