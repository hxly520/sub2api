#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
postgres_image="${POINTS_ACL_TEST_POSTGRES_IMAGE:-postgres:16-alpine}"
pg_host="${POINTS_ACL_TEST_PGHOST:-127.0.0.1}"
pg_port="${POINTS_ACL_TEST_PGPORT:-5432}"
pg_user="${POINTS_ACL_TEST_PGUSER:-points_test}"
pg_password="${POINTS_ACL_TEST_PGPASSWORD:-points_test}"
pg_database="${POINTS_ACL_TEST_PGDATABASE:-points_test}"
target_role="points_email_acl_ci"

psql=(docker run --rm --network host -i
  -e "PGPASSWORD=${pg_password}"
  "${postgres_image}" psql -X -v ON_ERROR_STOP=1
  -h "${pg_host}" -p "${pg_port}" -U "${pg_user}" -d "${pg_database}")

run_sql() {
  "${psql[@]}"
}

run_template() {
  local template="$1"
  {
    printf "\\set points_app_role '%s'\n" "${target_role}"
    cat "${template}"
  } | run_sql
}

assert_state() {
  local expect_email="$1"
  local expect_username="$2"
  cat <<SQL | run_sql
DO \$audit\$
DECLARE
  role_oid oid := (SELECT oid FROM pg_catalog.pg_roles WHERE rolname='${target_role}');
  table_oid oid := 'public.users'::regclass::oid;
  expected_columns text[] := ARRAY['id','deleted_at'];
  required_column text;
BEGIN
  IF ${expect_email} THEN
    expected_columns := pg_catalog.array_append(expected_columns, 'email');
  END IF;
  IF ${expect_username} THEN
    expected_columns := pg_catalog.array_append(expected_columns, 'username');
  END IF;
  IF role_oid IS NULL THEN
    RAISE EXCEPTION 'ACL test role is missing';
  END IF;
  FOREACH required_column IN ARRAY expected_columns LOOP
    IF NOT pg_catalog.has_column_privilege(
      '${target_role}', table_oid, required_column, 'SELECT'
    ) OR NOT EXISTS (
      SELECT 1
      FROM pg_catalog.pg_attribute attribute
      CROSS JOIN LATERAL pg_catalog.aclexplode(attribute.attacl) privilege
      WHERE attribute.attrelid=table_oid
        AND attribute.attname=required_column
        AND privilege.grantee=role_oid
        AND privilege.privilege_type='SELECT'
        AND NOT privilege.is_grantable
    ) THEN
      RAISE EXCEPTION 'missing direct SELECT on expected column %', required_column;
    END IF;
  END LOOP;
  IF pg_catalog.has_column_privilege('${target_role}', table_oid, 'email', 'SELECT')
       IS DISTINCT FROM ${expect_email} OR
     pg_catalog.has_column_privilege('${target_role}', table_oid, 'username', 'SELECT')
       IS DISTINCT FROM ${expect_username} OR
     pg_catalog.has_table_privilege('${target_role}', table_oid, 'SELECT') OR
     pg_catalog.has_table_privilege('${target_role}', table_oid, 'INSERT') OR
     pg_catalog.has_table_privilege('${target_role}', table_oid, 'UPDATE') OR
     pg_catalog.has_table_privilege('${target_role}', table_oid, 'DELETE') OR EXISTS (
       SELECT 1 FROM pg_catalog.pg_class relation
       CROSS JOIN LATERAL pg_catalog.aclexplode(relation.relacl) privilege
       WHERE relation.oid=table_oid AND privilege.grantee=role_oid
     ) OR EXISTS (
       SELECT 1 FROM pg_catalog.pg_attribute attribute
       WHERE attribute.attrelid=table_oid
         AND attribute.attnum > 0
         AND NOT attribute.attisdropped
         AND NOT (attribute.attname=ANY(expected_columns))
         AND pg_catalog.has_column_privilege(
           '${target_role}', table_oid, attribute.attnum, 'SELECT'
         )
     ) OR EXISTS (
       SELECT 1 FROM pg_catalog.pg_class relation
       CROSS JOIN LATERAL pg_catalog.aclexplode(relation.relacl) privilege
       WHERE relation.oid=table_oid AND privilege.grantee=0
     ) OR EXISTS (
       SELECT 1 FROM pg_catalog.pg_attribute attribute
       CROSS JOIN LATERAL pg_catalog.aclexplode(attribute.attacl) privilege
       WHERE attribute.attrelid=table_oid AND privilege.grantee=0
     ) THEN
    RAISE EXCEPTION 'unexpected effective or PUBLIC ACL state';
  END IF;
END
\$audit\$;
SQL
}

stage_a="${script_dir}/shared-database-users-email-upgrade.sql.example"
stage_b="${script_dir}/shared-database-users-email-finalize.sql.example"
rollback_prepare="${script_dir}/shared-database-users-email-rollback-prepare.sql.example"
rollback_finalize="${script_dir}/shared-database-users-email-rollback-finalize.sql.example"

cat <<SQL | run_sql
DO \$cleanup\$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='${target_role}') THEN
    EXECUTE 'DROP OWNED BY ${target_role}';
    EXECUTE 'DROP ROLE ${target_role}';
  END IF;
END
\$cleanup\$;
CREATE ROLE ${target_role} WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOREPLICATION NOBYPASSRLS NOINHERIT CONNECTION LIMIT 8;
GRANT USAGE ON SCHEMA public TO ${target_role};
REVOKE CREATE ON DATABASE ${pg_database} FROM ${target_role};
REVOKE CREATE ON SCHEMA public FROM ${target_role};
REVOKE ALL ON TABLE public.users FROM PUBLIC;
GRANT SELECT (id,username,deleted_at) ON TABLE public.users TO ${target_role};
SQL

assert_state false true

# Inject a failure after stage A has issued GRANT(email), before COMMIT. Closing
# the failed psql session must leave the exact legacy ACL intact.
if {
  printf "\\set points_app_role '%s'\n" "${target_role}"
  awk '{ if ($0 == "COMMIT;") print "SELECT 1/0;"; print }' "${stage_a}"
} | run_sql >/tmp/points-email-acl-expected-failure.log 2>&1; then
  echo "stage A unexpectedly committed after the injected failure" >&2
  exit 1
fi
assert_state false true

# An unexpected readable column must fail preflight without partially granting
# email. Remove the deliberate bad grant before exercising the success path.
printf 'GRANT SELECT (balance) ON TABLE public.users TO %s;\n' "${target_role}" | run_sql
if run_template "${stage_a}" >/tmp/points-email-acl-expected-preflight-failure.log 2>&1; then
  echo "stage A accepted an expanded legacy allowlist" >&2
  exit 1
fi
cat <<SQL | run_sql
DO \$audit\$
BEGIN
  IF pg_catalog.has_column_privilege('${target_role}','public.users','email','SELECT') THEN
    RAISE EXCEPTION 'failed stage A retained an email grant';
  END IF;
END
\$audit\$;
REVOKE SELECT (balance) ON TABLE public.users FROM ${target_role};
SQL
assert_state false true

printf 'GRANT SELECT (username) ON TABLE public.users TO %s WITH GRANT OPTION;\n' "${target_role}" | run_sql
if run_template "${stage_a}" >/tmp/points-email-acl-expected-grant-option-failure.log 2>&1; then
  echo "stage A accepted a delegable username grant" >&2
  exit 1
fi
printf 'REVOKE GRANT OPTION FOR SELECT (username) ON TABLE public.users FROM %s;\n' "${target_role}" | run_sql
assert_state false true

run_template "${stage_a}"
assert_state true true
run_template "${stage_b}"
assert_state true false
run_template "${rollback_prepare}"
assert_state true true
run_template "${rollback_finalize}"
assert_state false true

cat <<SQL | run_sql
DROP OWNED BY ${target_role};
DROP ROLE ${target_role};
SQL

echo "shared-database login-email ACL transition tests passed"
