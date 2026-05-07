import React, { useState, useEffect, useRef } from 'react';
import { Loader2, X } from 'lucide-react';
import { getDeleteJobStatus } from '@/lib/api';
import { useAuth } from '@/lib/auth-context';

interface DeleteProgressModalProps {
  jobId: string;
  isOpen: boolean;
  onClose: (finished: boolean) => void;
}

export default function DeleteProgressModal({ jobId, isOpen, onClose }: DeleteProgressModalProps) {
  const { token } = useAuth();
  const [job, setJob] = useState<any>(null);
  const [error, setError] = useState<string | null>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    if (!isOpen || !jobId || !token) return;

    const poll = async () => {
      try {
        const data = await getDeleteJobStatus(token, jobId);
        setJob(data);

        if (data.status === 'completed' || data.status === 'failed') {
          if (intervalRef.current) clearInterval(intervalRef.current);
          if (data.status === 'failed') setError(data.error || 'O\'chirish jarayonida xatolik yuz berdi');
        }
      } catch (err) {
        console.error('Polling error:', err);
      }
    };

    poll();
    intervalRef.current = setInterval(poll, 2000);

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [isOpen, jobId, token]);

  if (!isOpen) return null;

  const isFinished = job?.status === 'completed' || job?.status === 'failed';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="bg-brand-card border border-brand-border rounded-xl p-6 w-full max-w-md shadow-2xl">
        <div className="flex justify-between items-center mb-4">
          <h3 className="text-lg font-semibold text-white">O'chirish jarayoni</h3>
          {!isFinished && <Loader2 className="animate-spin text-brand-red" size={20} />}
        </div>

        {job ? (
          <div className="space-y-4">
            <div className="flex justify-between text-sm text-gray-400">
              <span>Status: <strong className="text-white capitalize">{job.status}</strong></span>
              <span>{job.progress}%</span>
            </div>
            
            <div className="w-full bg-brand-border h-2 rounded-full overflow-hidden">
              <div 
                className={`h-full transition-all duration-300 ${job.status === 'failed' ? 'bg-red-500' : 'bg-brand-red'}`}
                style={{ width: `${job.progress}%` }}
              />
            </div>

            <p className="text-xs text-gray-500">Bosqich: {job.current_step}</p>
            
            {error && <p className="text-red-400 text-sm mt-2">{error}</p>}

            {isFinished && (
              <button 
                onClick={() => onClose(true)}
                className="w-full bg-brand-red hover:bg-orange-700 text-white font-medium py-2 rounded-lg transition-colors"
              >
                Yopish
              </button>
            )}
          </div>
        ) : (
          <div className="flex justify-center py-8">
            <Loader2 className="animate-spin text-gray-500" size={32} />
          </div>
        )}
      </div>
    </div>
  );
}
