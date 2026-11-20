import { lookupKameoLoans } from "../../utils/kameo";

/**
 * Söker ett lånnummer hos Kameo och svarar med kortsiktig/långsiktig
 * klassificering. Inloggningen sker server-side med uppgifterna från 1Password.
 */
export default defineEventHandler(async (event) => {
  const body = await readBody<{ loanNumber?: string }>(event).catch(
    () => ({}) as any,
  );
  const loanNumber =
    typeof body?.loanNumber === "string" ? body.loanNumber : "";

  try {
    return await lookupKameoLoans(loanNumber);
  } catch (error: any) {
    throw createError({
      statusCode: 502,
      statusMessage: error?.message || "Kunde inte hämta uppgifter från Kameo.",
    });
  }
});
