/**
 * Atlas web - the workspace icon set.
 *
 * Stroke icons on a 24-unit grid, lifted from the workspace prototypes
 * (`images/Atlas Design – 0x`) so the shell and the screens draw the same
 * marks. Decorative by default (`aria-hidden`); give the surrounding control
 * the accessible name.
 */

import type { ReactNode, SVGProps } from "react";

const PATHS = {
  dashboard: (
    <>
      <rect x="3.5" y="3.5" width="7" height="8" rx="1.5" />
      <rect x="13.5" y="3.5" width="7" height="5" rx="1.5" />
      <rect x="13.5" y="11.5" width="7" height="9" rx="1.5" />
      <rect x="3.5" y="14.5" width="7" height="6" rx="1.5" />
    </>
  ),
  folder: <path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" />,
  board: (
    <>
      <rect x="3.5" y="4" width="5" height="15" rx="1.5" />
      <rect x="10" y="4" width="5" height="10" rx="1.5" />
      <rect x="16.5" y="4" width="4" height="12" rx="1.5" />
    </>
  ),
  bars: <path d="M5 20v-6M10 20V9M15 20v-9M20 20V5" />,
  alert: (
    <>
      <path d="M12 4 21 19.5H3z" />
      <path d="M12 10v4M12 17h.01" />
    </>
  ),
  calendar: (
    <>
      <rect x="3.5" y="5" width="17" height="15" rx="2" />
      <path d="M3.5 9.5h17M8 3v4M16 3v4" />
    </>
  ),
  calendarPlus: (
    <>
      <rect x="3.5" y="5" width="17" height="15" rx="2" />
      <path d="M3.5 9.5h17M8 3v4M16 3v4M12 13v4M10 15h4" />
    </>
  ),
  pulse: <path d="M3 12h4l2.5-6 5 12 2.5-6H21" />,
  clipboard: (
    <>
      <rect x="5" y="4.5" width="14" height="16" rx="2" />
      <path d="M9 4.5V3.5h6v1M8.5 10h7M8.5 14h7M8.5 18h4" />
    </>
  ),
  people: (
    <>
      <circle cx="9" cy="8" r="3.5" />
      <path d="M2.5 20c0-3.6 2.9-6 6.5-6s6.5 2.4 6.5 6M16 4.6a3.5 3.5 0 0 1 0 6.8M18 14.3c2 .8 3.5 2.8 3.5 5.7" />
    </>
  ),
  document: (
    <>
      <path d="M14 3.5H7a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8.5z" />
      <path d="M14 3.5v5h5M8.5 13h7M8.5 16.5h4" />
    </>
  ),
  card: (
    <>
      <rect x="3" y="6" width="18" height="13" rx="2.5" />
      <path d="M3 10h18M16 14.5h2" />
    </>
  ),
  server: (
    <>
      <rect x="3.5" y="4" width="17" height="7" rx="2" />
      <rect x="3.5" y="13" width="17" height="7" rx="2" />
      <path d="M7.5 7.5h.01M7.5 16.5h.01" />
    </>
  ),
  building: <path d="M4 20.5V6l8-3 8 3v14.5M9 20.5v-5h6v5M8.5 9.5h.01M12 9.5h.01M15.5 9.5h.01" />,
  pie: (
    <>
      <path d="M12 3.5a8.5 8.5 0 1 0 8.5 8.5H12z" />
      <path d="M15 3.8A8.6 8.6 0 0 1 20.2 9H15z" />
    </>
  ),
  sliders: (
    <>
      <path d="M4 7h10M18 7h2M4 17h4M12 17h8" />
      <circle cx="16" cy="7" r="2" />
      <circle cx="10" cy="17" r="2" />
    </>
  ),
  signOut: <path d="M14 4.5h4a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2h-4M10 16l-4-4 4-4M6 12h10" />,
  chevronRight: <path d="M9 6l6 6-6 6" />,
  chevronLeft: <path d="M15 6l-6 6 6 6" />,
  chevronDown: <path d="M6 9l6 6 6-6" />,
  bell: (
    <>
      <path d="M6 16.5V11a6 6 0 0 1 12 0v5.5l1.5 2h-15z" />
      <path d="M10 20.5a2 2 0 0 0 4 0" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  clock: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 7.5V12l3 2" />
    </>
  ),
  check: <path d="M5 12.5l4.5 4.5L19 7.5" />,
  arrowRight: <path d="M5 12h14M13 6l6 6-6 6" />,
  arrowLeft: <path d="M19 12H5M11 6l-6 6 6 6" />,
  shieldCheck: (
    <>
      <path d="M12 3l7.5 3v5.5c0 4.6-3.2 8.3-7.5 9.5-4.3-1.2-7.5-4.9-7.5-9.5V6z" />
      <path d="M9 12l2.2 2.2L15.5 10" />
    </>
  ),
  shield: <path d="M12 3l7.5 3v5.5c0 4.6-3.2 8.3-7.5 9.5-4.3-1.2-7.5-4.9-7.5-9.5V6z" />,
  user: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21c0-4 3.6-6.5 8-6.5s8 2.5 8 6.5" />
    </>
  ),
  eye: (
    <>
      <path d="M2.5 12S6 5 12 5s9.5 7 9.5 7-3.5 7-9.5 7-9.5-7-9.5-7z" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  search: (
    <>
      <circle cx="11" cy="11" r="6.5" />
      <path d="M20 20l-4.2-4.2" />
    </>
  ),
  more: <path d="M6 12h.01M12 12h.01M18 12h.01" strokeWidth={3} />,
  grip: <path d="M9 5h.01M9 12h.01M9 19h.01M15 5h.01M15 12h.01M15 19h.01" strokeWidth={3} />,
  printer: (
    <>
      <path d="M7 9.5V4h10v5.5" />
      <path d="M7 17H5a2 2 0 0 1-2-2v-3.5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2V15a2 2 0 0 1-2 2h-2" />
      <rect x="7" y="13.5" width="10" height="6.5" rx="1" />
    </>
  ),
  paperclip: <path d="M20 11.5l-7.8 7.8a5 5 0 0 1-7.1-7.1l8.2-8.2a3.3 3.3 0 0 1 4.7 4.7l-8.2 8.2a1.7 1.7 0 0 1-2.4-2.4l7.5-7.5" />,
  close: <path d="M6 6l12 12M18 6 6 18" />,
  slash: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M6 18 18 6" />
    </>
  ),
  signal: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M5.6 5.6a9 9 0 0 0 0 12.8M18.4 5.6a9 9 0 0 1 0 12.8" />
    </>
  ),
  monitor: (
    <>
      <rect x="3" y="4.5" width="18" height="12" rx="2" />
      <path d="M8 20h8M12 16.5V20" />
    </>
  ),
  menu: <path d="M4 7h16M4 12h16M4 17h16" />,
  link: <path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1" />,
  upload: <path d="M12 15.5V4.5M7.5 9 12 4.5 16.5 9M4.5 15v3a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-3" />,
  external: <path d="M14 4.5h5.5V10M19.5 4.5 11 13M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4" />,
  flag: <path d="M5 20.5V4.5M5 4.5h11l-2 4 2 4H5" />,
  github: (
    <>
      <path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.25 6.44-4.95 0-2.84-2.37-5.24-5.52-5.84-1.25-.36-2.62-.32-3.77.14-.63-.2-1.22-.5-1.84-.88-.42-.71-1.2-1.4-1.8-2.04-.09-.2-.16-.42-.2-.65.03-.07.05-.14.05-.21 0-.41-.16-.8-.5-1.07-.2-.19-.3-.4-.32-.66-.01-.11-.02-.2-.02-.3 0-.28.02-.56.07-.83.09-.58.36-1.1.69-1.56 0-.06.01-.11.01-.18 0-.34-.06-.68-.2-1 0-.09-.01-.17-.01-.25 0-.29-.02-.58-.07-.86 0-.11-.01-.2-.02-.31-.01-.3-.06-.6-.18-.87-.05-.15-.12-.3-.2-.44-.04-.08-.08-.17-.13-.25-.04-.05-.09-.11-.14-.16-.09-.1-.18-.21-.29-.3-.11-.11-.24-.2-.38-.27-.12-.06-.24-.12-.38-.16-.1-.03-.22-.06-.33-.08-.16-.02-.32-.03-.49-.04-.35 0-.68.02-1 .04-.18.01-.35.03-.53.04-.27.01-.54.03-.81.04-.17.01-.34.02-.51.05-.14.01-.28.03-.42.05-.2.03-.41.05-.61.06-.18.02-.37.04-.55.07-.15.02-.29.05-.44.08-.21.04-.42.09-.62.15-.11.03-.22.06-.33.1-.17.06-.35.13-.5.21-.14.08-.28.17-.4.27-.11.1-.22.22-.32.34-.08.09-.15.18-.21.27-.09.14-.16.29-.22.44-.03.09-.05.19-.07.28-.02.09-.05.18-.06.27-.01.09-.02.18-.02.27 0 .08 0 .16.01.23.01.14.02.27.03.41.02.25.04.5.04.75 0 .14-.01.28-.02.42-.02.28-.03.55-.04.83 0 .15-.01.29-.01.44-.01.25-.02.5-.03.75 0 .19-.01.37-.02.56 0 .12-.01.24-.01.36 0 .11-.01.22-.01.33-.01.22-.01.44-.02.66 0 .1-.01.19-.01.29-.01.19-.02.37-.02.56 0 .1-.01.2-.01.29 0 .1-.01.2-.01.3 0 .09-.01.17-.01.26 0 .05-.01.1-.01.15 0 .04-.01.08-.01.12s-.01.08-.01.11c0 .02-.01.05-.01.07 0 .05-.01.1-.01.14 0 .02-.01.05-.01.07 0 .04-.01.07-.01.11l-.01.03c0 .02-.01.03-.01.05l-.01.03c0 .02-.01.04-.01.06-.01.03-.01.07-.02.1l-.01.02c-.01.02-.01.05-.02.07-.01.02-.02.04-.03.06-.01.01-.02.03-.03.04-.01.01-.01.03-.02.04l-.02.03c-.01.02-.03.04-.04.06l-.01.01c-.01.01-.02.03-.03.04-.01.01-.01.02-.02.03s-.01.01-.02.02l-.01.01-.01.01c0 0-.01.01-.01.01v.01l-.01.01c0 0-.01 0-.01.01l-.01.01c0 0 0 .01 0 .01" />
    </>
  ),
} satisfies Record<string, ReactNode>;

export type IconName = keyof typeof PATHS;

export function Icon({
  name,
  className,
  ...props
}: { name: IconName } & Omit<SVGProps<SVGSVGElement>, "children">) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      focusable="false"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.7}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={["h-[18px] w-[18px] flex-none", className].filter(Boolean).join(" ")}
      {...props}
    >
      {PATHS[name]}
    </svg>
  );
}
