"use client";

import { usePathname, useRouter } from "next/navigation";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from "react";

import { Drawer } from "@/components/drawer";
import { Button, Select, Switch } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { Avatar, Badge, Tag, cx, priorityTone } from "@/components/ui";
import { BOARD_COLUMNS, columnLabel, shortRef } from "@/components/work/board";
import { NewTaskForm } from "@/components/work/new-task-form";
import { TaskDrawer, type DrawerFocus } from "@/components/work/task-drawer";
import { taskBody, updateTask, type Task, type TaskStatus } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import type { PersonOption } from "@/lib/directory";
import { dueLabel, formatShortDay, humanise, initials, isOverdue } from "@/lib/format";

/** Pixels a pointer must travel before a press becomes a drag. */
const DRAG_THRESHOLD = 6;
/** Distance from an edge at which the board and page scroll while dragging. */
const EDGE = 64;

interface DragState {
  id: string;
  pointerId: number;
  startX: number;
  startY: number;
  offsetX: number;
  offsetY: number;
  width: number;
  active: boolean;
}

export interface BoardFilters {
  projectId: string;
  /** A person id, `unassigned`, or empty for anyone. */
  assigneeId: string;
  overdue: boolean;
}

/**
 * The task board (blueprint §8.2 "project board"; prototype 03).
 *
 * Moving a card is a real write: `PUT /api/v1/work/tasks/{id}` with the
 * version it read in `If-Match`, so moves survive a reload and a stale card is
 * refused rather than overwriting someone else's change. The move shows at
 * once and is rolled back if Go says no; Go's reason is shown as it gave it.
 *
 * Dragging uses pointer events, so it works with a mouse (press anywhere on a
 * card), on touch screens (press the grip handle; the rest of the card still
 * scrolls the page) and with a pen. The grip is also a button: tapping or
 * pressing Enter on it opens a "Move to" menu, which is the keyboard path.
 *
 * Dropping into Blocked opens the card on its blocker form, because a blocked
 * task needs a reason. Dropping into Done is Go's decision: a task without
 * evidence is refused with `TASK_EVIDENCE_REQUIRED`, and the card opens on its
 * evidence section with Go's message.
 */
export function TaskBoard({
  initialTasks,
  projects,
  people,
  me,
  filters,
  cancelled,
  openNew,
}: {
  initialTasks: Task[];
  projects: Array<{ id: string; name: string }>;
  people: PersonOption[];
  me?: string;
  filters: BoardFilters;
  cancelled: number;
  openNew: boolean;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const notify = useToast();

  const [tasks, setTasks] = useState<Task[]>(initialTasks);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [focus, setFocus] = useState<DrawerFocus>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [newOpen, setNewOpen] = useState(openNew);
  const [moving, setMoving] = useState<Set<string>>(new Set());
  const [menuFor, setMenuFor] = useState<string | null>(null);

  const [dragId, setDragId] = useState<string | null>(null);
  const [overColumn, setOverColumn] = useState<string | null>(null);
  const [ghostStart, setGhostStart] = useState<{ x: number; y: number; width: number } | null>(null);
  const drag = useRef<DragState | null>(null);
  const over = useRef<string | null>(null);
  const ghost = useRef<HTMLDivElement>(null);
  const board = useRef<HTMLDivElement>(null);
  const suppressClick = useRef(false);

  // A fresh server render (filters, router.refresh) replaces the local copy.
  useEffect(() => setTasks(initialTasks), [initialTasks]);

  const names = useMemo(() => new Map(people.map((p) => [p.id, p.name])), [people]);
  const projectNames = useMemo(() => new Map(projects.map((p) => [p.id, p.name])), [projects]);

  const visible = useMemo(
    () =>
      filters.assigneeId === "unassigned"
        ? tasks.filter((task) => !task.assignee_person_id)
        : tasks,
    [tasks, filters.assigneeId],
  );

  const replace = useCallback((next: Task) => {
    setTasks((list) => list.map((task) => (task.id === next.id ? next : task)));
  }, []);

  const open = useCallback((task: Task, nextFocus: DrawerFocus = null, message: string | null = null) => {
    setSelectedId(task.id);
    setFocus(nextFocus);
    setNotice(message);
  }, []);

  const moveTo = useCallback(
    async (task: Task, status: TaskStatus) => {
      if (task.status === status) return;
      setMenuFor(null);
      setMoving((set) => new Set(set).add(task.id));
      replace({ ...task, status });
      try {
        const updated = await updateTask(task.id, taskBody(task, { status }), task.version);
        replace(updated);
        if (status === "blocked" && !updated.blocked) {
          open(updated, "blocker");
          notify("Moved to Blocked — add the reason so someone can unblock it");
        } else {
          if (selectedId === task.id) setNotice(null);
          notify(`Moved “${task.title}” to ${columnLabel(status)}`);
        }
      } catch (error) {
        replace(task);
        if (error instanceof ApiError && error.code === "TASK_EVIDENCE_REQUIRED") {
          open(task, "evidence", error.message);
        } else if (error instanceof ApiError) {
          notify(error.message, "error");
          // A stale version means someone else moved it: re-read the board.
          if (error.status === 412) router.refresh();
        } else {
          notify("The move didn’t reach Atlas. Check your connection and try again.", "error");
        }
      } finally {
        setMoving((set) => {
          const next = new Set(set);
          next.delete(task.id);
          return next;
        });
      }
    },
    [notify, open, replace, router, selectedId],
  );

  // ── pointer drag ──────────────────────────────────────────────────────────

  // The window listeners below are registered once; they read these.
  const latest = useRef({ tasks, moveTo });
  useEffect(() => {
    latest.current = { tasks, moveTo };
  }, [tasks, moveTo]);

  const endDrag = useCallback((commit: boolean) => {
    const state = drag.current;
    drag.current = null;
    document.body.style.userSelect = "";
    if (!state?.active) return;
    suppressClick.current = true;
    window.setTimeout(() => {
      suppressClick.current = false;
    }, 0);
    const target = over.current;
    over.current = null;
    setDragId(null);
    setOverColumn(null);
    setGhostStart(null);
    if (commit && target) {
      const task = latest.current.tasks.find((t) => t.id === state.id);
      if (task) void latest.current.moveTo(task, target as TaskStatus);
    }
  }, []);

  useEffect(() => {
    const onMove = (event: PointerEvent) => {
      const state = drag.current;
      if (!state || event.pointerId !== state.pointerId) return;
      if (!state.active) {
        if (Math.hypot(event.clientX - state.startX, event.clientY - state.startY) < DRAG_THRESHOLD) return;
        state.active = true;
        document.body.style.userSelect = "none";
        setMenuFor(null);
        setDragId(state.id);
        setGhostStart({
          x: event.clientX - state.offsetX,
          y: event.clientY - state.offsetY,
          width: state.width,
        });
      }
      event.preventDefault();
      if (ghost.current) {
        ghost.current.style.transform = `translate(${event.clientX - state.offsetX}px, ${event.clientY - state.offsetY}px) rotate(-1.5deg)`;
      }
      const column = document
        .elementFromPoint(event.clientX, event.clientY)
        ?.closest<HTMLElement>("[data-column]")?.dataset.column ?? null;
      if (column !== over.current) {
        over.current = column;
        setOverColumn(column);
      }
      const rect = board.current?.getBoundingClientRect();
      if (board.current && rect) {
        if (event.clientX > rect.right - EDGE) board.current.scrollLeft += 14;
        else if (event.clientX < rect.left + EDGE) board.current.scrollLeft -= 14;
      }
      if (event.clientY > window.innerHeight - EDGE) window.scrollBy(0, 14);
      else if (event.clientY < EDGE + 68) window.scrollBy(0, -14);
    };
    const onUp = (event: PointerEvent) => {
      if (drag.current && event.pointerId === drag.current.pointerId) endDrag(true);
    };
    const onCancel = (event: PointerEvent) => {
      if (drag.current && event.pointerId === drag.current.pointerId) endDrag(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && drag.current?.active) endDrag(false);
    };
    window.addEventListener("pointermove", onMove, { passive: false });
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onCancel);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onCancel);
      window.removeEventListener("keydown", onKey);
    };
  }, [endDrag]);

  function startDrag(event: ReactPointerEvent<HTMLElement>, task: Task, fromHandle: boolean) {
    if (event.button !== 0 || moving.has(task.id)) return;
    // On touch, only the grip starts a drag, so the card still scrolls.
    if (event.pointerType === "touch" && !fromHandle) return;
    const card = (event.currentTarget as HTMLElement).closest<HTMLElement>("[data-card]");
    if (!card) return;
    const rect = card.getBoundingClientRect();
    drag.current = {
      id: task.id,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      offsetX: event.clientX - rect.left,
      offsetY: event.clientY - rect.top,
      width: rect.width,
      active: false,
    };
  }

  // ── filters ───────────────────────────────────────────────────────────────

  function applyFilters(next: Partial<BoardFilters>) {
    const merged = { ...filters, ...next };
    const query = new URLSearchParams();
    if (merged.projectId) query.set("project_id", merged.projectId);
    if (merged.assigneeId) query.set("assignee_person_id", merged.assigneeId);
    if (merged.overdue) query.set("overdue", "true");
    const qs = query.toString();
    router.push(qs ? `${pathname}?${qs}` : pathname);
  }

  const filtered = Boolean(filters.projectId || filters.assigneeId || filters.overdue);
  const selected = tasks.find((task) => task.id === selectedId) ?? null;
  const dragged = tasks.find((task) => task.id === dragId);

  function closeNew() {
    setNewOpen(false);
    if (openNew) router.replace(pathname);
  }

  return (
    <>
      <div className="flex flex-wrap items-end gap-x-[18px] gap-y-3.5 rounded-card border border-line bg-white px-[18px] py-4 shadow-card">
        <FilterField label="Project" htmlFor="filter-project">
          <Select
            id="filter-project"
            value={filters.projectId}
            onChange={(event) => applyFilters({ projectId: event.target.value })}
          >
            <option value="">All projects</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </Select>
        </FilterField>
        <FilterField label="Owner" htmlFor="filter-assignee">
          <Select
            id="filter-assignee"
            value={filters.assigneeId}
            onChange={(event) => applyFilters({ assigneeId: event.target.value })}
          >
            <option value="">Anyone</option>
            <option value="unassigned">Unassigned</option>
            {me && <option value={me}>Me</option>}
            {people
              .filter((person) => person.id !== me)
              .map((person) => (
                <option key={person.id} value={person.id}>
                  {person.name}
                </option>
              ))}
            {filters.assigneeId &&
              filters.assigneeId !== "unassigned" &&
              filters.assigneeId !== me &&
              !names.has(filters.assigneeId) && (
                <option value={filters.assigneeId}>Person {filters.assigneeId.slice(0, 8)}</option>
              )}
          </Select>
        </FilterField>
        <Switch
          label="Overdue only"
          checked={filters.overdue}
          onChange={(event) => applyFilters({ overdue: event.target.checked })}
        />
        <div className="flex flex-1 flex-wrap items-center justify-end gap-2.5">
          {filtered && (
            <Button variant="ghost" onClick={() => router.push(pathname)}>
              Clear filters
            </Button>
          )}
          <Button onClick={() => setNewOpen(true)}>
            <Icon name="plus" />
            New task
          </Button>
        </div>
      </div>

      <p className="mt-5 flex items-start gap-2 text-[13.5px] text-muted">
        <Icon name="grip" className="mt-0.5 h-4 w-4" />
        <span>
          Drag a card to move it (on a touch screen, drag by its handle). Moving to Done needs
          evidence; moving to Blocked asks for a reason.
          {cancelled > 0 && ` ${cancelled} cancelled task${cancelled === 1 ? "" : "s"} hidden.`}
        </span>
      </p>

      <div
        ref={board}
        className="mt-3.5 grid grid-cols-[repeat(5,minmax(248px,1fr))] items-start gap-4 overflow-x-auto pb-2.5 scroll-quiet"
      >
        {BOARD_COLUMNS.map((column) => {
          const items = visible.filter((task) => task.status === column.status);
          const isOver = overColumn === column.status && dragged?.status !== column.status;
          return (
            <section
              key={column.status}
              data-column={column.status}
              aria-label={`${column.label}, ${items.length} task${items.length === 1 ? "" : "s"}`}
              className={cx(
                "flex min-h-[520px] flex-col gap-2.5 rounded-card border-[1.5px] p-3 transition-colors",
                isOver
                  ? "border-dashed border-atlas bg-[#E6EFFF]"
                  : "border-[#E3E6ED] bg-[#EEF0F5]",
              )}
            >
              <h2 className="flex items-center gap-2 px-1.5 pb-1.5 pt-1 text-[12.5px] font-semibold uppercase tracking-[0.07em] text-[#3B4A63]">
                <i aria-hidden="true" className="h-2.5 w-2.5 flex-none rounded-full" style={{ background: column.dot }} />
                {column.label}
                <span className="tabular ml-auto grid h-[22px] min-w-6 place-items-center rounded-full bg-white px-[7px] text-xs tracking-normal text-night">
                  {items.length}
                </span>
              </h2>

              {items.map((task) => (
                <TaskCard
                  key={task.id}
                  task={task}
                  owner={task.assignee_person_id ? names.get(task.assignee_person_id) : undefined}
                  isMe={Boolean(me && task.assignee_person_id === me)}
                  projectName={filters.projectId ? undefined : projectNames.get(task.project_id)}
                  dragging={dragId === task.id}
                  busy={moving.has(task.id)}
                  menuOpen={menuFor === task.id}
                  onMenu={(value) => {
                    if (!suppressClick.current) setMenuFor(value ? task.id : null);
                  }}
                  onPointerDown={(event, fromHandle) => startDrag(event, task, fromHandle)}
                  onOpen={() => {
                    if (!suppressClick.current) open(task);
                  }}
                  onMove={(status) => void moveTo(task, status)}
                />
              ))}

              {items.length === 0 && (
                <div className="grid min-h-[120px] place-items-center rounded-[14px] border-[1.5px] border-dashed border-[#C7CFDD] p-3 text-center text-[13.5px] text-[#6B7890]">
                  {dragId ? "Drop here" : filters.overdue ? "No overdue tasks" : "Nothing here yet"}
                </div>
              )}
            </section>
          );
        })}
      </div>

      {dragged && ghostStart && (
        <div
          ref={ghost}
          aria-hidden="true"
          className="pointer-events-none fixed left-0 top-0 z-50 opacity-95"
          style={{
            width: ghostStart.width,
            transform: `translate(${ghostStart.x}px, ${ghostStart.y}px) rotate(-1.5deg)`,
          }}
        >
          <TaskCardBody
            task={dragged}
            owner={dragged.assignee_person_id ? names.get(dragged.assignee_person_id) : undefined}
            isMe={Boolean(me && dragged.assignee_person_id === me)}
            className="cursor-grabbing shadow-pop"
          />
        </div>
      )}

      <TaskDrawer
        task={selected}
        projectName={selected ? projectNames.get(selected.project_id) : undefined}
        people={people}
        me={me}
        focus={focus}
        notice={notice}
        onClose={() => {
          setSelectedId(null);
          setFocus(null);
          setNotice(null);
        }}
        onMove={(task, status) => void moveTo(task, status)}
        onChanged={replace}
      />

      <Drawer
        open={newOpen}
        onClose={closeNew}
        title="New task"
        description="Owned, dated and tracked from the moment it exists."
      >
        <NewTaskForm
          projects={projects}
          people={people}
          me={me}
          defaultProjectId={filters.projectId}
          onCancel={closeNew}
          onCreated={(task) => {
            setTasks((list) => [task, ...list]);
            closeNew();
            notify(`Created “${task.title}”`);
            router.refresh();
          }}
        />
      </Drawer>
    </>
  );
}

function FilterField({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor: string;
  children: ReactNode;
}) {
  return (
    <label htmlFor={htmlFor} className="flex min-w-[200px] max-w-[280px] flex-[1_1_200px] flex-col gap-1.5">
      <span className="text-xs font-semibold uppercase tracking-[0.08em] text-muted">{label}</span>
      {children}
    </label>
  );
}

function TaskCard({
  task,
  owner,
  isMe,
  projectName,
  dragging,
  busy,
  menuOpen,
  onMenu,
  onPointerDown,
  onOpen,
  onMove,
}: {
  task: Task;
  owner?: string;
  isMe: boolean;
  projectName?: string;
  dragging: boolean;
  busy: boolean;
  menuOpen: boolean;
  onMenu: (open: boolean) => void;
  onPointerDown: (event: ReactPointerEvent<HTMLElement>, fromHandle: boolean) => void;
  onOpen: () => void;
  onMove: (status: TaskStatus) => void;
}) {
  const menu = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    const onPointer = (event: PointerEvent) => {
      if (!menu.current?.contains(event.target as Node)) onMenu(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onMenu(false);
    };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [menuOpen, onMenu]);

  return (
    <div className="relative" ref={menu}>
      <TaskCardBody
        task={task}
        owner={owner}
        isMe={isMe}
        projectName={projectName}
        className={cx(
          "cursor-grab active:cursor-grabbing",
          dragging && "opacity-40",
          busy && "animate-pulse",
        )}
        onPointerDown={(event) => onPointerDown(event, false)}
        onOpen={onOpen}
        grip={
          <button
            type="button"
            aria-label={`Move “${task.title}”`}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            onPointerDown={(event) => {
              event.stopPropagation();
              onPointerDown(event, true);
            }}
            onClick={() => onMenu(!menuOpen)}
            className="-m-1.5 grid h-9 w-9 flex-none touch-none place-items-center rounded-[10px] text-[#9AA6BA] transition hover:bg-[#F1F4FA] hover:text-night"
          >
            <Icon name="grip" />
          </button>
        }
      />
      {menuOpen && (
        <div
          role="menu"
          aria-label="Move to"
          className="absolute right-2 top-11 z-20 w-48 animate-fade-in rounded-[14px] border border-line bg-white p-1.5 shadow-pop"
        >
          <p className="px-2.5 pb-1 pt-1.5 text-[11px] font-semibold uppercase tracking-[0.09em] text-muted">
            Move to
          </p>
          {BOARD_COLUMNS.filter((column) => column.status !== task.status).map((column) => (
            <button
              key={column.status}
              type="button"
              role="menuitem"
              onClick={() => onMove(column.status)}
              className="flex w-full items-center gap-2.5 rounded-[10px] px-2.5 py-2 text-left text-sm text-night hover:bg-[#F3F6FC]"
            >
              <i aria-hidden="true" className="h-2 w-2 rounded-full" style={{ background: column.dot }} />
              {column.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function TaskCardBody({
  task,
  owner,
  isMe,
  projectName,
  className,
  grip,
  onPointerDown,
  onOpen,
}: {
  task: Task;
  owner?: string;
  isMe: boolean;
  projectName?: string;
  className?: string;
  grip?: ReactNode;
  onPointerDown?: (event: ReactPointerEvent<HTMLElement>) => void;
  onOpen?: () => void;
}) {
  const overdue = isOverdue(task.due_at, task.status);
  const needsReason = task.status === "blocked" && !task.blocked;
  const ref = shortRef(task.ref);
  const ownerName = owner ?? (task.assignee_person_id ? `Person ${task.assignee_person_id.slice(0, 8)}` : null);

  return (
    <article
      data-card
      onPointerDown={onPointerDown}
      className={cx(
        "flex select-none flex-col gap-2.5 rounded-[14px] border border-line bg-white p-3.5 shadow-tile transition hover:border-[#B9CEFA] hover:shadow-lift",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="flex flex-wrap items-center gap-1.5">
          <Badge tone={priorityTone(task.priority)}>{humanise(task.priority)}</Badge>
          {ref && <Tag>{ref}</Tag>}
        </span>
        {grip}
      </div>

      <button
        type="button"
        onClick={onOpen}
        className="text-left text-[15.5px] font-semibold leading-snug text-night hover:text-atlas-ink hover:underline hover:underline-offset-[3px]"
      >
        {task.title}
      </button>
      {projectName && <p className="-mt-1.5 truncate text-[12.5px] text-muted">{projectName}</p>}

      {task.blocked && (
        <p className="flex items-start gap-2 rounded-[10px] bg-[#FDECEC] px-2.5 py-2 text-[13px] font-medium text-[#B42318]">
          <Icon name="alert" className="mt-px h-[15px] w-[15px]" />
          <span className="line-clamp-2">{task.blocked_reason || "Blocked"}</span>
        </p>
      )}
      {needsReason && (
        <p className="flex items-center gap-2 rounded-[10px] border border-dashed border-[#F3A39C] px-2.5 py-[7px] text-[12.5px] font-semibold text-[#B42318]">
          <Icon name="alert" className="h-[15px] w-[15px]" />
          Blocked — reason needed
        </p>
      )}

      <div className="flex items-center justify-between gap-2">
        {ownerName ? (
          <span className="inline-flex min-w-0 items-center gap-2 text-[13px] text-muted">
            <Avatar initials={initials(ownerName)} size="sm" variant={isMe ? "me" : "default"} />
            <span className="truncate">{isMe ? "You" : ownerName}</span>
          </span>
        ) : (
          <span className="inline-flex items-center gap-2 text-[13px] text-muted">
            <Avatar initials="+" size="sm" variant="empty" />
            Unassigned
          </span>
        )}
        {task.due_at && (
          <span
            className={cx(
              "inline-flex items-center gap-1.5 whitespace-nowrap text-[12.5px] font-medium",
              overdue ? "text-[#C0352B]" : "text-muted",
            )}
            title={dueLabel(task.due_at) ?? undefined}
          >
            <Icon name={overdue ? "clock" : "calendar"} className="h-3.5 w-3.5" />
            {overdue ? dueLabel(task.due_at) : formatShortDay(task.due_at)}
          </span>
        )}
      </div>
    </article>
  );
}
