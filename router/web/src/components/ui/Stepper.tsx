import clsx from "clsx";

interface StepperProps {
  steps: string[];
  current: number;
}

export function Stepper({ steps, current }: StepperProps) {
  return (
    <div className="flex items-start mb-8">
      {steps.map((label, i) => (
        <div key={i} className="flex items-start flex-1 last:flex-none">
          <div className="flex flex-col items-center gap-1 flex-shrink-0">
            <div
              className={clsx(
                "w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-colors",
                i < current && "bg-blue-600 text-white",
                i === current && "bg-blue-600 text-white ring-2 ring-blue-200 ring-offset-1",
                i > current && "bg-gray-100 text-gray-400",
              )}
            >
              {i < current ? "✓" : i + 1}
            </div>
            <span
              className={clsx(
                "text-xs text-center whitespace-nowrap",
                i === current && "text-blue-600 font-medium",
                i !== current && "text-gray-400",
              )}
            >
              {label}
            </span>
          </div>
          {i < steps.length - 1 && (
            <div
              className={clsx(
                "flex-1 h-0.5 mt-4 mx-2",
                i < current ? "bg-blue-600" : "bg-gray-200",
              )}
            />
          )}
        </div>
      ))}
    </div>
  );
}
