// Browser dictation via the Web Speech API. Feature-detected: the mic button
// is hidden where the webview has no recognizer.

interface RecognitionLike {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  start(): void;
  stop(): void;
  onresult: ((event: { resultIndex: number; results: ArrayLike<{ isFinal: boolean; 0: { transcript: string } }> }) => void) | null;
  onerror: ((event: { error: string }) => void) | null;
  onend: (() => void) | null;
}

type RecognitionCtor = new () => RecognitionLike;

function ctor(): RecognitionCtor | undefined {
  const w = globalThis as unknown as { SpeechRecognition?: RecognitionCtor; webkitSpeechRecognition?: RecognitionCtor };
  return w.SpeechRecognition ?? w.webkitSpeechRecognition;
}

export function dictationSupported(): boolean {
  return typeof ctor() === "function";
}

export interface Dictation {
  stop(): void;
}

/**
 * Start dictating. onText receives the full text recognised so far in this
 * run (final + interim); onEnd fires once recognition stops for any reason.
 */
export function startDictation(handlers: {
  onText(text: string): void;
  onError(message: string): void;
  onEnd(): void;
}): Dictation | null {
  const Recognition = ctor();
  if (!Recognition) return null;
  const rec = new Recognition();
  rec.continuous = true;
  rec.interimResults = true;
  rec.lang = navigator.language || "en-US";
  let finalText = "";
  rec.onresult = (event) => {
    let interim = "";
    for (let i = event.resultIndex; i < event.results.length; i++) {
      const result = event.results[i];
      if (result.isFinal) finalText += result[0].transcript;
      else interim += result[0].transcript;
    }
    handlers.onText((finalText + interim).trim());
  };
  rec.onerror = (event) => {
    if (event.error !== "no-speech" && event.error !== "aborted") {
      handlers.onError(event.error === "not-allowed" ? "Microphone permission was denied" : `Dictation error: ${event.error}`);
    }
  };
  rec.onend = () => handlers.onEnd();
  try {
    rec.start();
  } catch (err) {
    handlers.onError(err instanceof Error ? err.message : "Could not start dictation");
    return null;
  }
  return { stop: () => rec.stop() };
}
