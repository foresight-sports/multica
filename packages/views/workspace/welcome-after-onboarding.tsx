"use client";

import { useEffect } from "react";
import { useWelcomeStore } from "@multica/core/onboarding";

/** Discard legacy skip signals. Workspace creation never seeds runtime guides. */
export function WelcomeAfterOnboarding() {
  const signal = useWelcomeStore((state) => state.signal);
  const reset = useWelcomeStore((state) => state.reset);
  useEffect(() => {
    if (signal) reset();
  }, [signal, reset]);
  return null;
}
