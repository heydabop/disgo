sqlite3 sqlite.db "select Content from Message where ChanId = '$1' and Content != '';" > ~/markov/chan_$1_custom
sed -i -r 's,<@.*?> ,,gm' ~/markov/chan_$1_custom
sed -i -r '/^\/.*/d' ~/markov/chan_$1_custom
sed -i -r 's,<@.*?>,,gm' ~/markov/chan_$1_custom
sed -i -r '/^$/d' ~/markov/chan_$1_custom
