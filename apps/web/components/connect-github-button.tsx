"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";

import { Button } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { getGitHubStatus } from "@/lib/api/atlas";
import { cx } from "@/components/ui";

/**
 * ConnectGitHubButton — a pop-up installation icon in the top bar.
 *
 * Shows a "Connect GitHub" prompt when the person has not linked their GitHub
 * account. After connecting, it shows a success state and links to the
 * integrations page.
 */

export function ConnectGitHubButton() {
  const router = useRouter();
  const notify = useToast();
  const [connected, setConnected] = useState<boolean | null>(null);
  const [checking, setChecking] = useState(true);

  const checkStatus = useCallback(async () => {
    try {
      const status = await fetch("/api/v1/integrations/github/status", {
        credentials: "include",
        cache: "no-store",
      });
      if (status.ok) {
        const data = await status.json();
        setConnected(!!data.connected);
      } else {
        setConnected(false);
      }
    } catch {
      setConnected(false);
    } finally {
      setChecking(false);
    }
  }, []);

  useEffect(() => {
    checkStatus();
  }, [checkStatus]);

  if (checking) return null;
  if (connected) return null;

  return (
    <Button
      type="button"
      size="sm"
      variant="secondary"
      onClick={() => {
        router.push("/integrations/github");
        notify("Connect your GitHub account to see code activity in Engineering Intelligence.");
      }}
      className="h-8 px-2 text-[12.5px] gap-1.5"
    >
      <Icon name="github" className="h-3.5 w-3.5" />
      Connect GitHub
    </Button>
  );
}
