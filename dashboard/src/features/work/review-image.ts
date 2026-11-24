export const reviewImageNote = "Numbered preview areas and reviewer comments.";

const reviewImageReference =
  /^Review screenshot \d+: (work-requests\/followup-[a-f0-9]{64}-screenshot-[1-4]\.jpeg)\r?\nNote: Numbered preview areas and reviewer comments\.$/m;

export function submittedReviewImage(instruction: string) {
  return reviewImageReference.exec(instruction)?.[1];
}

export function reviewMessageText(instruction: string) {
  return instruction.replace(reviewImageReference, "").trimEnd();
}
