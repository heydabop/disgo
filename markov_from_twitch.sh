cp ~/irclogs/Twitch/\#$1.log ~/markov/$1_nolink
sed -i -r 's,^--- Log opened .*$,,gm' ~/markov/$1_nolink
sed -i -r 's,^--- Log closed .*$,,gm' ~/markov/$1_nolink
sed -i -r 's,^..:.. .* has joined #'$1'$,,gm' ~/markov/$1_nolink
sed -i -r 's,..:.. < .*?> ,,gm' ~/markov/$1_nolink
sed -i -r 's,^..:.. -!- ServerMode\/.*$,,gm' ~/markov/$1_nolink
sed -i -r 's,\.([[:alpha:]]+)\/,DOT\1,gm' ~/markov/$1_nolink
sed -i -r '/^$/d' ~/markov/$1_nolink
