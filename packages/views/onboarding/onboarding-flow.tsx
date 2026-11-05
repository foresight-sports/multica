"use client";

import { useCallback, useState } from "react";
import { WorkspaceDirectory } from "../workspace/workspace-directory";
import { api } from "@multica/core/api";
import { toast } from "sonner";
import { useAuthStore } from "@multica/core/auth";
import {
  completeOnboarding,
  ONBOARDING_STEP_ORDER,
  saveQuestionnaire,
  useWelcomeStore,
  type OnboardingStep,
  type QuestionnaireAnswers,
} from "@multica/core/onboarding";
import { useWorkspaceList } from "@multica/core/workspace";
import type { Workspace } from "@multica/core/types";
import { StepWelcome } from "./steps/step-welcome";
import { StepShell } from "./components/step-shell";

import { StepAboutYou } from "./steps/step-about-you";
import { StepWorkspace } from "./steps/step-workspace";
import { OnboardingLogoutButton } from "./components/onboarding-logout-button";
import { useT } from "../i18n";

const EMPTY_QUESTIONNAIRE: QuestionnaireAnswers = {
  source: [],
  source_other: null,
  source_skipped: false,
  role: null,
  role_other: null,
  role_skipped: false,
  use_case: [],
  use_case_other: null,
  use_case_skipped: false,
  version: 2,
};

/**
 * Coerce a stored questionnaire slot into the array shape used by the
 * current UI. Earlier versions of this app wrote `source` / `use_case`
 * as a single string; tolerate that on read so a user who started
 * onboarding before this change doesn't see their previous answer
 * disappear on re-entry. Empty string and null both collapse to [].
 */
function coerceToArray<T extends string>(value: unknown): T[] {
  if (Array.isArray(value)) {
    return value.filter((v): v is T => typeof v === "string" && v.length > 0);
  }
  if (typeof value === "string" && value.length > 0) {
    return [value as T];
  }
  return [];
}

/**
 * Merge persisted answers into the empty default. Re-entry pre-fills
 * answered slots but treats `*_skipped` as fresh (the user can answer
 * this time) — the v1 skip marker is dropped on read, the analytics
 * record of the prior skip stays in the DB.
 */
function mergeQuestionnaire(
  raw: Record<string, unknown>,
): QuestionnaireAnswers {
  const merged = {
    ...EMPTY_QUESTIONNAIRE,
    ...(raw as Partial<QuestionnaireAnswers>),
  };
  return {
    ...merged,
    source: coerceToArray<QuestionnaireAnswers["source"][number]>(raw.source),
    use_case: coerceToArray<QuestionnaireAnswers["use_case"][number]>(
      raw.use_case,
    ),
    source_skipped: false,
    role_skipped: false,
    use_case_skipped: false,
  };
}

/** Workspace creation finishes onboarding without provisioning runtime content. */
interface OnboardingFlowProps {
  onComplete: (
    workspace?: Workspace,
    destination?: OnboardingDestination,
  ) => void;
  /** "new_workspace" is the same flow run by someone who already uses
   *  Multica: it starts at the workspace step, because the intro and the
   *  questionnaire only make sense once per person, and it always creates a
   *  workspace rather than offering to continue with an existing one. */
  mode?: OnboardingMode;
  /** Required in "new_workspace" mode: first-run onboarding has no way out
   *  except signing out, but someone creating a second workspace must be able
   *  to change their mind and return to where they were. */
  onCancel?: () => void;
  runtimeInstructions?: React.ReactNode;
  /** Retained for shell compatibility; onboarding no longer configures runtimes. */
  onRuntimeRefresh?: () => void | Promise<void>;
  runtimesPending?: boolean;
}

export function OnboardingFlow(props: OnboardingFlowProps) {
  return <OnboardingStepFlow {...props} />;
}

function OnboardingStepFlow({
  onComplete,
  mode = "first_run",
  onCancel,
  runtimeInstructions,
}: OnboardingFlowProps) {
  const { t } = useT("onboarding");
  const user = useAuthStore((s) => s.user);
  if (!user) {
    throw new Error("OnboardingFlow requires an authenticated user");
  }

  // Questionnaire answers are server-persisted and pre-fill the per-
  // question steps on re-entry. That's the only piece of onboarding
  // state persisted across sessions — which step the user is on is
  // deliberately not saved, so every entry starts at Welcome.
  const storedQuestionnaire = mergeQuestionnaire(user.onboarding_questionnaire);
  const [answers, setAnswers] =
    useState<QuestionnaireAnswers>(storedQuestionnaire);

  const [createRequested, setCreateRequested] = useState(false);
  const isNewWorkspace = mode === "new_workspace";
  const isWeb = !!runtimeInstructions;
  const [step, setStep] = useState<OnboardingStep>(
    isNewWorkspace || isWeb ? "workspace" : "welcome",
  );
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  // Raised by whichever step has a request in flight; locks Back and the
  // rail. Only the workspace step sets it today.
  const [stepBusy, setStepBusy] = useState(false);
  const [completing, setCompleting] = useState(false);

  // Fetched at Step 0 + Step 2. Step 2 uses it to detect a pre-existing
  // workspace from an earlier abandoned onboarding (so StepWorkspace shows
  // "Continue with {name}" instead of CreateWorkspaceForm — avoiding the
  // slug conflict that creation would hit). Step 0 uses it to decide
  // whether to render the "I've done this before" skip button — only
  // shown when the user already has at least one workspace, otherwise
  // skipping would land them in limbo.
  const { workspaces, ready: workspacesReady } = useWorkspaceList({
    enabled: step === "welcome" || step === "workspace",
  });
  const existingWorkspace = isNewWorkspace
    ? workspace
    : (workspace ?? workspaces[0] ?? null);
  const canSkipWelcome = workspacesReady && workspaces.length > 0;

  // Derive "what comes after `from`" from ONBOARDING_STEP_ORDER so
  // inserting/reordering a persisted step only requires editing the
  // canonical array. Returns null if `from` is the last persisted step
  // or not in the array (callers fall back to bespoke routing).
  const nextStep = useCallback(
    (from: OnboardingStep): OnboardingStep | null => {
      const idx = ONBOARDING_STEP_ORDER.indexOf(from);
      if (idx < 0 || idx >= ONBOARDING_STEP_ORDER.length - 1) return null;
      return ONBOARDING_STEP_ORDER[idx + 1]!;
    },
    [],
  );

  const advanceFrom = useCallback(
    (from: OnboardingStep) => {
      const next = nextStep(from);
      if (next) setStep(next);
    },
    [nextStep],
  );

  const handleWelcomeNext = useCallback(() => {
    // Welcome is intentionally not in ONBOARDING_STEP_ORDER (it's a
    // product intro, not a persisted step), so the first persisted
    // step is hard-coded as the entry point.
    setStep(ONBOARDING_STEP_ORDER[0]!);
  }, []);

  // Apply an in-memory patch and fire-and-forget a PATCH to persist
  // it. We never block UI on the request — the next step's render is
  // what matters; a transient save failure surfaces as a toast but
  // does not roll the user back.
  const applyAnswers = useCallback((patch: Partial<QuestionnaireAnswers>) => {
    setAnswers((a) => {
      const next = { ...a, ...patch };
      void saveQuestionnaire(next).catch((err) => {
        if (err instanceof Error) toast.error(err.message);
      });
      return next;
    });
  }, []);

  // "I've done this before" path — returning user who already has a
  // workspace and just wants to land there. Marks onboarding complete
  // server-side (idempotent via COALESCE on onboarded_at) and navigates
  // without creating new workspace content.
  const handleWelcomeSkip = useCallback(async () => {
    try {
      await completeOnboarding("skip_existing", workspaces[0]?.id);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t(($) => $.errors.skip_failed),
      );
      return;
    }
    onComplete(workspaces[0] ?? undefined);
  }, [workspaces, onComplete, t]);

  const openExistingWorkspace = async (ws: Workspace) => {
    if (stepBusy) return;
    setStepBusy(true);
    try {
      await completeOnboarding("skip_existing", ws.id);
      onComplete(ws);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : t(($) => $.errors.skip_failed),
      );
      setStepBusy(false);
    }
  };

  const handleWorkspaceCreated = useCallback(
    async (ws: Workspace) => {
      setWorkspace(ws);
      setCompleting(true);
      try {
        await completeOnboarding("runtime_skipped", ws.id);
        useWelcomeStore.getState().reset();
        onComplete(ws);
      } catch (err) {
        toast.error(
          err instanceof Error ? err.message : t(($) => $.errors.skip_failed),
        );
      } finally {
        setCompleting(false);
      }
    },
    [onComplete, t],
  );

  const handleBack = useCallback(
    (from: OnboardingStep) => {
      // The workspace step is the entry point in new-workspace mode, so its
      // back affordance leaves the flow instead of walking into a step this
      // mode deliberately skips. Back is hidden once the workspace exists.
      if (isNewWorkspace && from === "workspace") {
        onCancel?.();
        return;
      }
      const idx = ONBOARDING_STEP_ORDER.indexOf(from);
      if (idx <= 0) {
        // About you (the first persisted step) returns to Welcome.
        setStep("welcome");
        return;
      }
      const prev = ONBOARDING_STEP_ORDER[idx - 1]!;
      setStep(prev);
    },
    [isNewWorkspace, onCancel],
  );

  const headerTrailing = <OnboardingLogoutButton inline />;

  const handleStepChange = isNewWorkspace || isWeb ? undefined : setStep;

  // New accounts discover teams before creation; the API decides whether the
  // creation button is available. Approval never provisions runtime or agents.
  if (
    step === "workspace" &&
    !workspace &&
    (!isNewWorkspace || user.permissions?.create_workspaces !== true) &&
    !createRequested
  ) {
    return (
      <div className="mx-auto w-full max-w-3xl space-y-6 p-6 sm:p-10">
        <WorkspaceDirectory
          busy={stepBusy}
          onCreate={() => setCreateRequested(true)}
          onOpen={(id) => {
            setStepBusy(true);
            void api
              .getWorkspace(id)
              .then(openExistingWorkspace)
              .catch(() => toast.error(t(($) => $.workspace_entry.retry)))
              .finally(() => setStepBusy(false));
          }}
        />
        <OnboardingLogoutButton inline />
      </div>
    );
  }
  // ONE shell for the whole flow, rendered here rather than by each step.
  // Every step used to render its own <StepShell>, and because each step is a
  // different component type React tore the shell down and built a new one on
  // every transition — remounting the "persistent" rail, restarting its canvas,
  // and replaying the shell's fade from opacity 0. That full-window re-fade is
  // the flash. Hoisting the shell makes the rail actually persistent and leaves
  // only the step body swapping.
  if (step === "welcome") {
    // Welcome has no rail, so the escape hatch stays pinned there.
    return (
      <>
        <OnboardingLogoutButton />
        <StepWelcome
          onNext={handleWelcomeNext}
          onSkip={canSkipWelcome ? handleWelcomeSkip : undefined}
          isWeb={isWeb}
        />
      </>
    );
  }

  const stepBack =
    step === "about_you"
      ? () => handleBack("about_you")
      : step === "workspace"
        ? isWeb && !isNewWorkspace
          ? () => setCreateRequested(false)
          : () => handleBack("workspace")
        : undefined;

  return (
    <StepShell
      currentStep={step}
      onBack={workspace ? undefined : stepBack}
      backDisabled={stepBusy}
      onStepChange={workspace ? undefined : handleStepChange}
      chromeFooter={headerTrailing}
    >
      {step === "about_you" && (
        <StepAboutYou
          answers={answers}
          onChange={applyAnswers}
          onAdvance={() => advanceFrom("about_you")}
          onSkip={() => advanceFrom("about_you")}
        />
      )}

      {step === "workspace" && (
        <StepWorkspace
          key={workspace?.id ?? "create"}
          existing={existingWorkspace}
          created={!!workspace}
          completing={completing}
          onCreated={handleWorkspaceCreated}
          onBusyChange={workspace ? undefined : setStepBusy}
        />
      )}
    </StepShell>
  );
}

export type OnboardingMode = "first_run" | "new_workspace";

export type OnboardingDestination =
  | { kind: "issue"; issueId: string }
  | {
      kind: "chat";
      sessionId: string;
    };

export type { OnboardingStep };
