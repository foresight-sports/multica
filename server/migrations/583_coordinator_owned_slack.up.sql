ALTER TABLE instance_configuration ADD COLUMN audience_instructions text NOT NULL DEFAULT $guidance$The coordinator determines the intended audience from the request and conversation. If the audience or required expertise is unclear, request clarification or escalate to the configured human; do not invent a route or assume an unavailable agent exists.
Customer support: provide the finding, supported workaround or precise clarifying questions directly to the CS representative. Do not supply sample customer emails, scripts, greetings or sign-offs.
Sales: explain verified capabilities, practical impact and limitations. Do not invent capabilities, commercial commitments or timelines.
Internal teams: provide relevant evidence, technical context and actionable next steps.
For every audience, preserve material uncertainty. Use a communications specialist when useful; the coordinator owns the final response. State that a defect was added to the backlog only after the issue exists, and never promise a delivery date.$guidance$;
ALTER TABLE slack_intake_rule ALTER COLUMN analyst_id DROP NOT NULL;
