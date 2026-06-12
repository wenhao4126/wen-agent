import type { Todo } from "./tools";

export function shouldShowTodoPanel(
  todoId: string | null | undefined,
  dismissedTodoId: string | null,
  todos: Todo[],
): boolean {
  if (!todoId || todos.length === 0) return false;
  // All todos are done — no reason to show the panel even if
  // dismissedTodoId doesn't match the current batch.
  const hasActive = todos.some((t) => t.status !== "completed");
  if (!hasActive) return false;
  // User dismissed this exact batch of todos — suppress until a
  // new todo_write call changes the id.
  return todoId !== dismissedTodoId;
}
