"use client";

import Image from "next/image";
import { useRouter } from "next/navigation";
import QRCode from "qrcode";
import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type InputHTMLAttributes,
  type ReactNode,
} from "react";

import { ApiError } from "@/lib/api/client";
import {
  MFA_CHALLENGE_TTL_MS,
  activateMfa,
  enrollMfa,
  login,
  verifyMfa,
} from "@/lib/auth/flow";
import logo from "@/public/brand/safaristack-logo.png";

/**
 * The Atlas sign-in journey (docs/FRONTEND.md §2) as a small state machine:
 *
 *   signin ──200 {user}──────────────────────────────▶ done → /dashboard
 *     │
 *     └─200 {requires_mfa}─┬─ enrollment_required ─▶ enroll ─▶ recovery-codes ─▶ done
 *                          └─ otherwise ───────────▶ verify ─▶ done
 *
 * Go owns every rule; this component only collects input, calls `/api/*` and
 * reacts to the stable problem `code`. The `mfa_token` challenge lives in state
 * only and is dropped the moment it is used or expires.
 */

/** Stop accepting codes a little before Go's 5-minute challenge expires. */
const CHALLENGE_SAFETY_MS = 10_000;

interface Challenge {
  token: string;
  expiresAt: number;
}

type Step =
  | { kind: "signin"; notice?: string }
  | { kind: "verify"; challenge: Challenge; recovery: boolean }
  | { kind: "enroll"; challenge: Challenge; secret: string; qr: string }
  | { kind: "recovery-codes"; codes: string[] }
  | { kind: "forgot" }
  | { kind: "done"; name?: string };

interface Problem {
  message: string;
  reference?: string;
}

const EXPIRED_NOTICE =
  "Your verification window closed after 5 minutes. Sign in again to get a new one.";

// ── input rules (presentation only; Go decides) ──────────────

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function identifierError(raw: string): string {
  const value = raw.trim();
  if (!value) return "Enter your email address or phone number.";
  if (value.includes("@")) {
    return EMAIL.test(value) ? "" : "Enter a valid email address, e.g. you@safaristack.co.ke.";
  }
  const digits = value.replace(/\D/g, "");
  const phoneLike = /^[\d\s()+-]+$/.test(value);
  return phoneLike && digits.length >= 9 && digits.length <= 15
    ? ""
    : "Enter a valid email address or phone number, e.g. 0712 345 678.";
}

/** Recovery codes are 16 base32 characters; Go ignores case and dashes. */
function normaliseRecoveryCode(raw: string): string {
  return raw.replace(/[\s-]/g, "").toUpperCase();
}

function newChallenge(token: string): Challenge {
  return { token, expiresAt: Date.now() + MFA_CHALLENGE_TTL_MS - CHALLENGE_SAFETY_MS };
}

function isExpired(challenge: Challenge): boolean {
  return Date.now() >= challenge.expiresAt;
}

/** Map a failure to words a person can act on. Switch on `code`, never `detail`. */
function describe(error: unknown, context: "credentials" | "code"): Problem {
  if (!(error instanceof ApiError)) {
    return { message: "Atlas can’t be reached right now. Check your connection and try again." };
  }
  switch (error.code) {
    case "UNAUTHENTICATED":
      return {
        message:
          context === "credentials"
            ? "Those details don’t match an active Atlas account."
            : "That code didn’t work. Check your authenticator app and try again.",
      };
    case "RATE_LIMITED":
      return { message: "Too many attempts. Wait a few minutes before trying again." };
    case "VALIDATION_FAILED":
      return {
        message:
          context === "credentials"
            ? "Check your details and try again."
            : "Enter the code exactly as shown.",
      };
    default:
      return {
        message:
          error.status >= 500 || !error.code
            ? "Something went wrong on our side. Try again in a moment."
            : (error.problem.detail ?? "Something went wrong. Try again."),
        reference: error.requestId,
      };
  }
}

async function qrDataUrl(otpauthUrl: string): Promise<string> {
  // Rendered locally: the TOTP secret must never reach a third-party QR service.
  return QRCode.toDataURL(otpauthUrl, {
    errorCorrectionLevel: "M",
    margin: 1,
    width: 416,
    color: { dark: "#001F4D", light: "#FFFFFF" },
  });
}

// ── the flow ─────────────────────────────────────────────────

export function LoginFlow() {
  const router = useRouter();
  const [step, setStep] = useState<Step>({ kind: "signin" });
  const [identifier, setIdentifier] = useState("");
  const container = useRef<HTMLDivElement>(null);
  const firstRender = useRef(true);

  const focusKey = step.kind === "verify" ? `verify:${step.recovery}` : step.kind;
  useEffect(() => {
    // Move focus to the new step's first control so keyboard and screen
    // reader users land where the work is. Not on first paint: phones would
    // pop the keyboard over the artwork before the person has looked.
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }
    container.current?.querySelector<HTMLElement>("[data-autofocus]")?.focus();
  }, [focusKey]);

  function backToSignin(notice?: string) {
    setStep({ kind: "signin", notice });
  }

  function finish(name?: string) {
    setStep({ kind: "done", name });
    router.replace("/dashboard");
  }

  async function startEnrollment(challenge: Challenge) {
    const { secret, otpauth_url } = await enrollMfa(challenge.token);
    setStep({ kind: "enroll", challenge, secret, qr: await qrDataUrl(otpauth_url) });
  }

  const compact = step.kind === "enroll" || step.kind === "recovery-codes";

  return (
    <div ref={container} className="flex flex-col items-center">
      <Image
        src={logo}
        alt="SafariStack Solutions"
        priority
        sizes="(min-width: 640px) 192px, 160px"
        className={
          compact
            ? "h-auto w-24 sm:w-28 [@media(max-height:560px)]:w-20"
            : "h-auto w-40 sm:w-48 [@media(max-height:560px)]:w-28"
        }
      />

      <div className="mt-3 w-full sm:mt-4">
        {step.kind === "signin" && (
          <SigninStep
            identifier={identifier}
            onIdentifier={setIdentifier}
            notice={step.notice}
            onForgot={() => setStep({ kind: "forgot" })}
            onSignedIn={finish}
            onChallenge={async (token, enrollmentRequired) => {
              const challenge = newChallenge(token);
              if (enrollmentRequired) {
                await startEnrollment(challenge);
              } else {
                setStep({ kind: "verify", challenge, recovery: false });
              }
            }}
          />
        )}

        {step.kind === "verify" && (
          <VerifyStep
            key={step.recovery ? "recovery" : "totp"}
            challenge={step.challenge}
            recovery={step.recovery}
            onToggleRecovery={() => setStep({ ...step, recovery: !step.recovery })}
            onBack={() => backToSignin()}
            onExpired={() => backToSignin(EXPIRED_NOTICE)}
            onNotEnrolled={() => startEnrollment(step.challenge)}
            onSignedIn={finish}
          />
        )}

        {step.kind === "enroll" && (
          <EnrollStep
            challenge={step.challenge}
            secret={step.secret}
            qr={step.qr}
            onBack={() => backToSignin()}
            onExpired={() => backToSignin(EXPIRED_NOTICE)}
            onAlreadyActive={() =>
              setStep({ kind: "verify", challenge: step.challenge, recovery: false })
            }
            onActivated={(codes) => setStep({ kind: "recovery-codes", codes })}
          />
        )}

        {step.kind === "recovery-codes" && (
          <RecoveryCodesStep codes={step.codes} onContinue={() => finish()} />
        )}

        {step.kind === "forgot" && <ForgotStep onBack={() => backToSignin()} />}

        {step.kind === "done" && <DoneStep name={step.name} />}
      </div>
    </div>
  );
}

// ── steps ────────────────────────────────────────────────────

function SigninStep(props: {
  identifier: string;
  onIdentifier: (value: string) => void;
  notice?: string;
  onForgot: () => void;
  onSignedIn: (name?: string) => void;
  onChallenge: (token: string, enrollmentRequired: boolean) => Promise<void>;
}) {
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [idError, setIdError] = useState("");
  const [pwError, setPwError] = useState("");
  const [problem, setProblem] = useState<Problem | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const nextIdError = identifierError(props.identifier);
    const nextPwError = password ? "" : "Enter your password.";
    setIdError(nextIdError);
    setPwError(nextPwError);
    setProblem(null);
    if (nextIdError || nextPwError) return;

    setBusy(true);
    try {
      const response = await login(props.identifier.trim(), password);
      if (response.requires_mfa && response.mfa_token) {
        await props.onChallenge(response.mfa_token, Boolean(response.enrollment_required));
        return;
      }
      props.onSignedIn(response.user?.name);
    } catch (error) {
      setProblem(describe(error, "credentials"));
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate aria-busy={busy} className="flex flex-col">
      <Heading>Welcome back</Heading>
      <Lead>Sign in to Atlas to continue</Lead>

      {props.notice && <Notice>{props.notice}</Notice>}

      <div className="mt-6 flex flex-col gap-3.5">
        <Field
          id="atl-identifier"
          label="Email or phone number"
          icon={<UserIcon />}
          error={idError}
          data-autofocus
          type="text"
          inputMode="email"
          autoComplete="username"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          placeholder="Email or phone number"
          value={props.identifier}
          onChange={(event) => {
            props.onIdentifier(event.target.value);
            setIdError("");
          }}
        />
        <Field
          id="atl-password"
          label="Password"
          icon={<LockIcon />}
          error={pwError}
          type={showPassword ? "text" : "password"}
          autoComplete="current-password"
          placeholder="Password"
          value={password}
          onChange={(event) => {
            setPassword(event.target.value);
            setPwError("");
          }}
          trailing={
            <button
              type="button"
              onClick={() => setShowPassword((shown) => !shown)}
              aria-label={showPassword ? "Hide password" : "Show password"}
              aria-pressed={showPassword}
              className="flex size-11 flex-none items-center justify-center rounded-xl text-[#DFE8FA] transition hover:bg-white/10 focus-visible:outline focus-visible:outline-2 focus-visible:outline-[#8CC4FF]"
            >
              {showPassword ? <EyeOffIcon /> : <EyeIcon />}
            </button>
          }
        />
      </div>

      <FormProblem problem={problem} />

      <PrimaryButton busy={busy} busyLabel="Signing in…">
        Sign in
      </PrimaryButton>

      <div className="mt-1 flex justify-end">
        <TextButton onClick={props.onForgot}>Forgot password?</TextButton>
      </div>
    </form>
  );
}

function VerifyStep(props: {
  challenge: Challenge;
  recovery: boolean;
  onToggleRecovery: () => void;
  onBack: () => void;
  onExpired: () => void;
  onNotEnrolled: () => Promise<void>;
  onSignedIn: (name?: string) => void;
}) {
  const [code, setCode] = useState("");
  const [codeError, setCodeError] = useState("");
  const [problem, setProblem] = useState<Problem | null>(null);
  const [busy, setBusy] = useState(false);
  const secondsLeft = useChallengeCountdown(props.challenge, props.onExpired);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const value = props.recovery ? normaliseRecoveryCode(code) : code;
    const error = props.recovery
      ? /^[A-Z2-7]{16}$/.test(value)
        ? ""
        : "Recovery codes are 16 letters and numbers."
      : /^\d{6}$/.test(value)
        ? ""
        : "Enter all 6 digits.";
    setCodeError(error);
    setProblem(null);
    if (error) return;
    if (isExpired(props.challenge)) {
      props.onExpired();
      return;
    }

    setBusy(true);
    try {
      const response = await verifyMfa(props.challenge.token, value);
      props.onSignedIn(response.user?.name);
    } catch (error) {
      if (error instanceof ApiError && error.code === "MFA_REQUIRED") {
        // The account has no active authenticator after all: enrol instead.
        try {
          await props.onNotEnrolled();
        } catch (enrollError) {
          setProblem(describe(enrollError, "code"));
          setBusy(false);
        }
        return;
      }
      setProblem(describe(error, "code"));
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate aria-busy={busy} className="flex flex-col">
      <Heading>Verify it’s you</Heading>
      <Lead>
        {props.recovery
          ? "Enter one of the recovery codes you saved when you set up two-step verification. Each code works once."
          : "Enter the 6-digit code from your authenticator app."}
      </Lead>
      <Countdown seconds={secondsLeft} />

      <div className="mt-5">
        {props.recovery ? (
          <Field
            id="atl-code"
            label="Recovery code"
            icon={<KeyIcon />}
            error={codeError}
            data-autofocus
            type="text"
            autoComplete="off"
            autoCapitalize="characters"
            autoCorrect="off"
            spellCheck={false}
            maxLength={24}
            placeholder="Recovery code"
            value={code}
            onChange={(event) => {
              setCode(event.target.value.toUpperCase());
              setCodeError("");
            }}
            inputClassName="font-mono tracking-[0.12em]"
          />
        ) : (
          <Field
            id="atl-code"
            label="6-digit code"
            icon={<ShieldIcon />}
            error={codeError}
            data-autofocus
            type="text"
            inputMode="numeric"
            pattern="[0-9]*"
            autoComplete="one-time-code"
            maxLength={6}
            placeholder="6-digit code"
            value={code}
            onChange={(event) => {
              setCode(event.target.value.replace(/\D/g, "").slice(0, 6));
              setCodeError("");
            }}
            inputClassName="tracking-[0.35em] placeholder:tracking-normal"
          />
        )}
      </div>

      <FormProblem problem={problem} />

      <PrimaryButton busy={busy} busyLabel="Verifying…">
        Verify
      </PrimaryButton>

      <div className="mt-1 flex flex-wrap items-center justify-between gap-x-4">
        <TextButton onClick={props.onBack}>Back to sign in</TextButton>
        <TextButton onClick={props.onToggleRecovery}>
          {props.recovery ? "Use authenticator code" : "Use a recovery code"}
        </TextButton>
      </div>
    </form>
  );
}

function EnrollStep(props: {
  challenge: Challenge;
  secret: string;
  qr: string;
  onBack: () => void;
  onExpired: () => void;
  onAlreadyActive: () => void;
  onActivated: (codes: string[]) => void;
}) {
  const [code, setCode] = useState("");
  const [codeError, setCodeError] = useState("");
  const [problem, setProblem] = useState<Problem | null>(null);
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const secondsLeft = useChallengeCountdown(props.challenge, props.onExpired);
  const groupedSecret = props.secret.match(/.{1,4}/g)?.join(" ") ?? props.secret;

  async function copySecret() {
    try {
      await navigator.clipboard.writeText(props.secret);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const error = /^\d{6}$/.test(code) ? "" : "Enter the 6-digit code your app shows.";
    setCodeError(error);
    setProblem(null);
    if (error) return;
    if (isExpired(props.challenge)) {
      props.onExpired();
      return;
    }

    setBusy(true);
    try {
      const { recovery_codes } = await activateMfa(props.challenge.token, code);
      props.onActivated(recovery_codes);
    } catch (error) {
      if (error instanceof ApiError && error.code === "MFA_ALREADY_ACTIVE") {
        props.onAlreadyActive();
        return;
      }
      setProblem(describe(error, "code"));
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate aria-busy={busy} className="flex flex-col">
      <Heading compact>Set up two-step verification</Heading>
      <Lead>
        Your role requires an authenticator app. Scan the code with Google Authenticator,
        Microsoft Authenticator or 1Password.
      </Lead>
      <Countdown seconds={secondsLeft} />

      <div className="mt-4 flex flex-col items-center gap-3 sm:flex-row sm:gap-5">
        <div className="flex-none rounded-2xl bg-white p-2 shadow-[0_0_0_6px_rgba(40,120,255,0.18)]">
          <Image
            src={props.qr}
            alt="QR code to add Atlas to your authenticator app"
            width={152}
            height={152}
            unoptimized
            className="size-36 sm:size-[9.5rem]"
          />
        </div>
        <div className="flex min-w-0 flex-col items-center text-center sm:items-start sm:text-left">
          <p className="text-sm text-ink-muted">Can’t scan it? Enter this key instead:</p>
          <code className="mt-1.5 select-all break-words rounded-lg bg-white/10 px-2.5 py-1.5 font-mono text-sm leading-relaxed tracking-wider text-white">
            {groupedSecret}
          </code>
          <TextButton onClick={copySecret}>{copied ? "Copied" : "Copy key"}</TextButton>
        </div>
      </div>

      <div className="mt-4">
        <Field
          id="atl-enroll-code"
          label="6-digit code from your app"
          icon={<ShieldIcon />}
          error={codeError}
          data-autofocus
          type="text"
          inputMode="numeric"
          pattern="[0-9]*"
          autoComplete="one-time-code"
          maxLength={6}
          placeholder="6-digit code from your app"
          value={code}
          onChange={(event) => {
            setCode(event.target.value.replace(/\D/g, "").slice(0, 6));
            setCodeError("");
          }}
          inputClassName="tracking-[0.35em] placeholder:tracking-normal"
        />
      </div>

      <FormProblem problem={problem} />

      <PrimaryButton busy={busy} busyLabel="Turning on…">
        Turn on and continue
      </PrimaryButton>

      <div className="mt-1 flex justify-center">
        <TextButton onClick={props.onBack}>Back to sign in</TextButton>
      </div>
    </form>
  );
}

function RecoveryCodesStep(props: { codes: string[]; onContinue: () => void }) {
  const [saved, setSaved] = useState(false);
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">("idle");
  const text = [
    "Atlas recovery codes",
    "Each code signs you in once if you lose your authenticator app.",
    "",
    ...props.codes,
    "",
  ].join("\n");

  useEffect(() => {
    // The codes are shown exactly once; warn before they are navigated away.
    if (saved) return;
    const warn = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [saved]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  }

  function download() {
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "atlas-recovery-codes.txt";
    link.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="flex flex-col">
      <Heading compact>Save your recovery codes</Heading>
      <Lead>
        Two-step verification is on. If you lose your phone, each code below signs you in
        once. This is the only time Atlas shows them.
      </Lead>

      <ol
        aria-label="Recovery codes"
        className="mt-4 grid grid-cols-2 gap-x-3 gap-y-1.5 rounded-2xl border-[1.5px] border-atlas-line bg-ink-field px-4 py-3 font-mono text-[0.95rem] tracking-wider text-white"
      >
        {props.codes.map((code) => (
          <li key={code} className="select-all text-center">
            {code}
          </li>
        ))}
      </ol>

      <div className="mt-1 flex flex-wrap items-center justify-center gap-x-6">
        <TextButton onClick={copy} data-autofocus>
          {copyState === "copied" ? "Copied" : "Copy codes"}
        </TextButton>
        <TextButton onClick={download}>Download .txt</TextButton>
      </div>
      {copyState === "failed" && (
        <p role="alert" className="text-center text-sm text-atlas-danger-text">
          Copying isn’t available here. Select the codes or download them instead.
        </p>
      )}

      <label className="mt-2 flex min-h-11 cursor-pointer items-center gap-3 text-[0.95rem] text-ink-muted">
        <input
          type="checkbox"
          checked={saved}
          onChange={(event) => setSaved(event.target.checked)}
          className="size-5 flex-none cursor-pointer accent-atlas focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[3px] focus-visible:outline-[#8CC4FF]"
        />
        I’ve stored these codes somewhere safe
      </label>

      <PrimaryButton type="button" disabled={!saved} onClick={props.onContinue}>
        Continue to Atlas
      </PrimaryButton>
    </div>
  );
}

function ForgotStep(props: { onBack: () => void }) {
  return (
    <div className="flex flex-col">
      <Heading>Forgot your password?</Heading>
      <Lead>
        Atlas doesn’t offer self-service password resets yet. Ask a SafariStack system
        administrator to help you regain access.
      </Lead>
      <PrimaryButton type="button" onClick={props.onBack} data-autofocus>
        Back to sign in
      </PrimaryButton>
    </div>
  );
}

function DoneStep(props: { name?: string }) {
  return (
    <div className="flex flex-col items-center">
      <div className="mt-2 flex size-16 items-center justify-center rounded-full bg-atlas shadow-[0_0_0_8px_rgba(40,120,255,0.18)]">
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" className="size-9">
          <path d="M5 12.5l4.5 4.5L19 7.5" />
        </svg>
      </div>
      <div className="mt-4">
        <Heading>{props.name ? `Welcome, ${props.name.split(" ")[0]}` : "You’re signed in"}</Heading>
      </div>
      <p role="status" className="mt-2 text-center text-ink-muted">
        Opening your Atlas workspace…
      </p>
    </div>
  );
}

// ── hooks ────────────────────────────────────────────────────

/** Seconds left on an MFA challenge; calls `onExpired` once when it runs out. */
function useChallengeCountdown(challenge: Challenge, onExpired: () => void): number {
  const [now, setNow] = useState(() => Date.now());
  const expiredRef = useRef(onExpired);
  useEffect(() => {
    expiredRef.current = onExpired;
  });

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const seconds = Math.max(0, Math.ceil((challenge.expiresAt - now) / 1000));
  useEffect(() => {
    if (seconds === 0) expiredRef.current();
  }, [seconds]);
  return seconds;
}

// ── building blocks ──────────────────────────────────────────

function Heading({ children, compact = false }: { children: ReactNode; compact?: boolean }) {
  return (
    <h1
      className={`text-center font-semibold leading-tight tracking-[-0.01em] ${compact ? "text-[1.5rem] sm:text-[1.75rem]" : "text-[1.75rem] sm:text-[2rem] xl:text-[2.25rem]"}`}
    >
      {children}
    </h1>
  );
}

function Lead({ children }: { children: ReactNode }) {
  return (
    <p className="mx-auto mt-1.5 max-w-[30em] text-center text-[0.95rem] leading-snug text-ink-muted sm:text-base">
      {children}
    </p>
  );
}

function Notice({ children }: { children: ReactNode }) {
  return (
    <p role="status" className="mt-4 rounded-xl border border-atlas-line/60 bg-white/10 px-3.5 py-2.5 text-center text-sm text-white">
      {children}
    </p>
  );
}

function Countdown({ seconds }: { seconds: number }) {
  const minutes = Math.floor(seconds / 60);
  const rest = String(seconds % 60).padStart(2, "0");
  return (
    <p
      className={`mt-1.5 text-center text-xs tabular-nums ${seconds <= 60 ? "text-atlas-danger-text" : "text-ink-hint"}`}
    >
      This step closes in {minutes}:{rest}
    </p>
  );
}

function FormProblem({ problem }: { problem: Problem | null }) {
  if (!problem) return null;
  return (
    <div role="alert" className="mt-4 rounded-xl border border-atlas-danger/60 bg-[#3A0F1A]/40 px-3.5 py-2.5 text-sm text-atlas-danger-text">
      <p>{problem.message}</p>
      {problem.reference && (
        <p className="mt-1 font-mono text-[0.7rem] text-atlas-danger-text/80">
          Reference {problem.reference}
        </p>
      )}
    </div>
  );
}

type FieldProps = InputHTMLAttributes<HTMLInputElement> & {
  id: string;
  label: string;
  icon: ReactNode;
  error?: string;
  trailing?: ReactNode;
  inputClassName?: string;
  "data-autofocus"?: boolean;
};

function Field({ id, label, icon, error, trailing, inputClassName = "", ...input }: FieldProps) {
  const errorId = `${id}-error`;
  return (
    <div>
      <div
        className={`flex h-[3.25rem] w-full items-center gap-3 rounded-2xl border-[1.5px] bg-ink-field pl-4 shadow-[0_0_22px_rgba(40,120,255,0.16)] transition focus-within:border-atlas-focus focus-within:shadow-[0_0_0_3px_rgba(80,160,255,0.35),0_0_22px_rgba(40,120,255,0.2)] sm:h-14 ${trailing ? "pr-1.5" : "pr-4"} ${error ? "border-atlas-danger" : "border-atlas-line"}`}
      >
        <span aria-hidden="true" className="flex-none text-[#DFE8FA]">
          {icon}
        </span>
        <label htmlFor={id} className="sr-only">
          {label}
        </label>
        <input
          id={id}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? errorId : undefined}
          className={`h-full min-w-0 flex-1 border-0 bg-transparent text-base text-white outline-none placeholder:text-ink-hint sm:text-[1.0625rem] ${inputClassName}`}
          {...input}
        />
        {trailing}
      </div>
      {error && (
        <p id={errorId} role="alert" className="ml-4 mt-1.5 text-sm text-atlas-danger-text">
          {error}
        </p>
      )}
    </div>
  );
}

function PrimaryButton({
  busy = false,
  busyLabel,
  children,
  type = "submit",
  ...rest
}: {
  busy?: boolean;
  busyLabel?: string;
  children: ReactNode;
  type?: "submit" | "button";
  disabled?: boolean;
  onClick?: () => void;
  "data-autofocus"?: boolean;
}) {
  return (
    <button
      type={type}
      {...rest}
      disabled={busy || rest.disabled}
      className="mt-5 h-[3.25rem] w-full rounded-2xl bg-atlas bg-gradient-to-b from-white/25 to-white/0 text-lg font-semibold text-white shadow-[0_8px_24px_rgba(20,100,255,0.35)] transition hover:brightness-110 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[3px] focus-visible:outline-[#8CC4FF] disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:brightness-100 aria-busy:cursor-progress sm:h-14"
      aria-busy={busy || undefined}
    >
      {busy && busyLabel ? busyLabel : children}
    </button>
  );
}

function TextButton({
  children,
  onClick,
  ...rest
}: {
  children: ReactNode;
  onClick: () => void;
  "data-autofocus"?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      {...rest}
      className="min-h-11 rounded-md text-[0.95rem] text-atlas-link underline underline-offset-[3px] transition hover:text-atlas-link-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[3px] focus-visible:outline-[#8CC4FF]"
    >
      {children}
    </button>
  );
}

// ── icons (stroke icons from the prototype) ──────────────────

function Icon({ children }: { children: ReactNode }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" className="size-[1.375rem]">
      {children}
    </svg>
  );
}

function UserIcon() {
  return (
    <Icon>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21c0-4 3.6-6.5 8-6.5s8 2.5 8 6.5" />
    </Icon>
  );
}

function LockIcon() {
  return (
    <Icon>
      <rect x="5" y="10.5" width="14" height="10" rx="2" />
      <path d="M8 10.5V7.5a4 4 0 0 1 8 0v3" />
      <path d="M12 14.5v2.5" />
    </Icon>
  );
}

function ShieldIcon() {
  return (
    <Icon>
      <path d="M12 3l7.5 3v5.5c0 4.6-3.2 8.3-7.5 9.5-4.3-1.2-7.5-4.9-7.5-9.5V6L12 3z" />
      <path d="M9 12l2.2 2.2L15.5 10" />
    </Icon>
  );
}

function KeyIcon() {
  return (
    <Icon>
      <circle cx="8" cy="15" r="4" />
      <path d="M10.8 12.2L20 3" />
      <path d="M17 6l3 3" />
    </Icon>
  );
}

function EyeIcon() {
  return (
    <Icon>
      <path d="M2.5 12S6 5 12 5s9.5 7 9.5 7-3.5 7-9.5 7-9.5-7-9.5-7z" />
      <circle cx="12" cy="12" r="3" />
    </Icon>
  );
}

function EyeOffIcon() {
  return (
    <Icon>
      <path d="M3 3l18 18" />
      <path d="M10.6 5.1A10.5 10.5 0 0 1 12 5c6 0 9.5 7 9.5 7a17 17 0 0 1-3 3.9" />
      <path d="M6.6 6.6C3.9 8.4 2.5 12 2.5 12s3.5 7 9.5 7a9.7 9.7 0 0 0 5.4-1.6" />
      <path d="M9.9 9.9a3 3 0 0 0 4.2 4.2" />
    </Icon>
  );
}
